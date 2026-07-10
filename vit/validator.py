"""Post-merge validation — shim over the Go `vit` binary.

The validation logic lives in Go (internal/vit/validator.go); this module
keeps the ValidationIssue dataclass and function signatures for the Resolve
plugin scripts.
"""

from dataclasses import dataclass, field
from typing import List

from .core import _call


@dataclass
class ValidationIssue:
    severity: str  # "error" or "warning"
    category: str  # "orphaned_ref", "sync", "overlap", "track_count"
    message: str
    details: dict = field(default_factory=dict)

    def __str__(self) -> str:
        icon = "ERROR" if self.severity == "error" else "WARN"
        return f"[{icon}] {self.category}: {self.message}"


def validate_project(project_dir: str) -> List[ValidationIssue]:
    """Run all validation checks on the current project state.

    Returns a list of issues found. Empty list = valid.
    """
    raw = _call("validate", project_dir=project_dir) or []
    return [
        ValidationIssue(
            severity=i.get("severity", ""),
            category=i.get("category", ""),
            message=i.get("message", ""),
            details=i.get("details") or {},
        )
        for i in raw
    ]


def format_issues(issues: List[ValidationIssue]) -> str:
    """Format validation issues for display."""
    if not issues:
        return "  No issues found."

    lines = []
    errors = [i for i in issues if i.severity == "error"]
    warnings = [i for i in issues if i.severity == "warning"]

    if errors:
        lines.append(f"  {len(errors)} error(s):")
        for issue in errors:
            lines.append(f"    {issue}")

    if warnings:
        lines.append(f"  {len(warnings)} warning(s):")
        for issue in warnings:
            lines.append(f"    {issue}")

    return "\n".join(lines)
