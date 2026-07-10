"""Timeline diffing — shim over the Go `vit` binary.

The diff logic (human-readable diffs, categorized changes) lives in Go
(internal/vit/differ.go). Only the functions the Resolve plugin uses are
exposed here; the full diff formatter is available via `vit diff` on the CLI.
"""

from typing import Dict, List, Tuple

from .core import _call


def get_changes_by_category(project_dir: str, ref: str = "HEAD") -> Dict[str, List[dict]]:
    """Get changes categorized by domain (audio, video, color).

    Returns a dict with keys: audio, video, color
    Each value is a list of dicts with: id, name, type (added/removed/modified), details
    """
    result = _call("changes-by-category", project_dir=project_dir, ref=ref)
    return {
        "audio": result.get("audio") or [],
        "video": result.get("video") or [],
        "color": result.get("color") or [],
    }


def get_branch_diff_by_category(
    project_dir: str, branch_a: str, branch_b: str
) -> Tuple[Dict[str, List[dict]], Dict[str, List[dict]]]:
    """Compare two branches and return categorized changes for each.

    Returns (changes_a, changes_b) where each is a dict with audio/video/color keys.
    """
    result = _call(
        "branch-diff-by-category",
        project_dir=project_dir,
        branch_a=branch_a,
        branch_b=branch_b,
    )
    return result.get("changes_a") or {}, result.get("changes_b") or {}
