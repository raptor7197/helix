"""AI-assisted analysis — shim over the Go `vit` binary.

Gemini calls, prompts and fallback heuristics live in Go
(internal/vit/aimerge.go). The panel only uses these two enrichment
functions; interactive AI conflict resolution runs in the CLI (`vit merge`).
Both degrade gracefully in Go when GEMINI_API_KEY is absent, so the GUI
never blocks on AI.
"""

from typing import Dict, List

from .core import _call, categorize_commit


def analyze_branch_comparison(
    branch_a: str,
    branch_b: str,
    changes_a: Dict[str, list],
    changes_b: Dict[str, list],
) -> dict:
    """Analyze two branches and provide a merge recommendation."""
    return _call(
        "analyze-branch-comparison",
        branch_a=branch_a,
        branch_b=branch_b,
        changes_a=changes_a,
        changes_b=changes_b,
    )


def classify_commit_type(
    commit_hash: str,
    files_changed: List[str],
    message: str = "",
) -> str:
    """Classify a commit's primary category: "audio", "video", or "color"."""
    try:
        return _call(
            "classify-commit",
            hash=commit_hash,
            files=files_changed,
            message=message,
        )
    except Exception:
        return categorize_commit(files_changed)
