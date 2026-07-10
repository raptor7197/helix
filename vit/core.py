"""Git wrapper — thin shim over the Go `vit` binary.

All git/diff/merge/AI logic lives in the Go binary (cmd/vit); this module
keeps the original function signatures so the Resolve plugin scripts keep
working unchanged. Each call shells out to `vit internal <op>` with a JSON
payload on stdin and a JSON response on stdout.
"""

import json
import os
import shutil
import subprocess
import sys
from typing import List, Optional, Tuple


class GitError(Exception):
    """Raised when a git command fails."""
    pass


_BINARY = None


def _find_binary() -> str:
    """Locate the Go vit binary: $VIT_BINARY, PATH, then ~/.vit/bin."""
    global _BINARY
    if _BINARY:
        return _BINARY

    exe = "vit.exe" if sys.platform == "win32" else "vit"
    candidates = []
    env = os.environ.get("VIT_BINARY")
    if env:
        candidates.append(env)
    which = shutil.which("vit")
    if which:
        candidates.append(which)
    candidates.append(os.path.expanduser(os.path.join("~", ".vit", "bin", exe)))

    for candidate in candidates:
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            _BINARY = candidate
            return _BINARY

    raise GitError(
        "vit binary not found. Install it (see README) or set VIT_BINARY to its path."
    )


def _call(op: str, **payload):
    """Run `vit internal <op>` and return the result, raising GitError on failure."""
    result = subprocess.run(
        [_find_binary(), "internal", op],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        raise GitError(f"vit internal {op} failed: {detail}")
    try:
        response = json.loads(result.stdout)
    except json.JSONDecodeError:
        raise GitError(f"vit internal {op} returned invalid JSON: {result.stdout[:200]}")
    if not response.get("ok"):
        raise GitError(response.get("error", f"vit internal {op} failed"))
    return response.get("result")


def git_add(project_dir: str, paths: List[str]) -> None:
    """Stage files for commit."""
    _call("add", project_dir=project_dir, paths=paths)


def git_commit(project_dir: str, message: str) -> str:
    """Create a commit. Returns the commit hash."""
    return _call("commit", project_dir=project_dir, message=message) or ""


def git_branch(project_dir: str, branch_name: str) -> None:
    """Create and switch to a new branch."""
    _call("branch", project_dir=project_dir, name=branch_name)


def git_checkout(project_dir: str, ref: str) -> None:
    """Switch to a branch or commit."""
    _call("checkout", project_dir=project_dir, ref=ref)


def git_merge(project_dir: str, branch: str) -> Tuple[bool, str]:
    """Attempt to merge a branch. Returns (success, output)."""
    result = _call("merge", project_dir=project_dir, branch=branch)
    return result["success"], result["output"]


def git_current_branch(project_dir: str) -> str:
    """Get current branch name."""
    return _call("current-branch", project_dir=project_dir)


def git_list_branches(project_dir: str) -> List[str]:
    """List all local branches."""
    return _call("list-branches", project_dir=project_dir) or []


def git_show_file(project_dir: str, ref: str, filepath: str) -> Optional[str]:
    """Get file content at a specific ref (e.g. 'HEAD', 'main', merge base)."""
    return _call("show-file", project_dir=project_dir, ref=ref, path=filepath)


def git_list_conflicted_files(project_dir: str) -> List[str]:
    """List files with merge conflicts."""
    return _call("list-conflicted", project_dir=project_dir) or []


def git_checkout_theirs(project_dir: str, paths: List[str]) -> None:
    """Resolve conflicts by taking the incoming branch's version."""
    _call("checkout-theirs", project_dir=project_dir, paths=paths)


def git_is_clean(project_dir: str) -> bool:
    """Check if working directory is clean."""
    return bool(_call("is-clean", project_dir=project_dir))


def git_push(project_dir: str, remote: str = "origin", branch: Optional[str] = None) -> str:
    """Push to remote."""
    return _call("push", project_dir=project_dir, remote=remote, branch=branch or "") or ""


def git_pull(project_dir: str, remote: str = "origin", branch: Optional[str] = None) -> str:
    """Pull from remote."""
    return _call("pull", project_dir=project_dir, remote=remote, branch=branch or "") or ""


def git_status(project_dir: str) -> str:
    """Get status output."""
    return _call("status", project_dir=project_dir) or ""


def git_log(project_dir: str, max_count: int = 20) -> str:
    """Get formatted log output."""
    return _call("log", project_dir=project_dir, max_count=max_count) or ""


def git_log_with_changes(project_dir: str, max_count: int = 20) -> List[dict]:
    """Get commit log with file change information for each commit."""
    return _call("log-with-changes", project_dir=project_dir, max_count=max_count) or []


def git_log_with_topology(project_dir: str, max_count: int = 30) -> dict:
    """Get commit log with parent information for graph visualization."""
    return _call("log-with-topology", project_dir=project_dir, max_count=max_count) or {
        "commits": [], "branches": [], "head": ""
    }


def find_project_root(start_dir: Optional[str] = None) -> Optional[str]:
    """Find the vit project root by looking for .vit/ directory.

    Pure filesystem walk — kept local to avoid a subprocess per lookup.
    """
    current = start_dir or os.getcwd()
    while True:
        if os.path.isdir(os.path.join(current, ".vit")):
            return current
        parent = os.path.dirname(current)
        if parent == current:
            return None
        current = parent


def categorize_commit(files_changed: List[str]) -> str:
    """Determine the dominant category for a commit based on files changed.

    Pure function, duplicated in Go (CategorizeCommit) — kept local as the
    plugin's no-subprocess fallback path.
    """
    counts = {"audio": 0, "video": 0, "color": 0}
    for f in files_changed:
        if "audio" in f.lower():
            counts["audio"] += 1
        elif "color" in f.lower():
            counts["color"] += 1
        elif "cuts" in f.lower() or "video" in f.lower():
            counts["video"] += 1
    max_cat = max(counts, key=counts.get)
    if counts[max_cat] == 0:
        return "video"
    return max_cat
