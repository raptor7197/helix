"""Merge policy helpers — shim over the Go `vit` binary.

The overlay-merge logic lives in Go (internal/vit/mergeutils.go); this module
keeps the OverlayMergePlan dataclass and function signatures for
resolve_plugin/vit_merge.py.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict, Tuple

from .core import _call


@dataclass
class OverlayMergePlan:
    """Bookkeeping for title-overlay remaps created during merge."""

    id_remaps: Dict[str, str] = field(default_factory=dict)
    generator_renames: Dict[str, str] = field(default_factory=dict)
    grade_renames: Dict[str, str] = field(default_factory=dict)
    grade_restore_ours: set[str] = field(default_factory=set)


def merge_timeline_domains_for_overlays(
    merged_files: Dict[str, dict],
    ours_files: Dict[str, dict],
    theirs_files: Dict[str, dict],
) -> Tuple[Dict[str, dict], OverlayMergePlan]:
    """Normalize title/media ID collisions so titles become overlays."""
    result = _call(
        "overlay-merge",
        merged=merged_files,
        ours=ours_files,
        theirs=theirs_files,
    )
    raw_plan = result.get("plan", {})
    plan = OverlayMergePlan(
        id_remaps=raw_plan.get("id_remaps") or {},
        generator_renames=raw_plan.get("generator_renames") or {},
        grade_renames=raw_plan.get("grade_renames") or {},
        grade_restore_ours=set(raw_plan.get("grade_restore_ours") or []),
    )
    return result.get("merged", {}), plan


def referenced_sidecars(domain_files: Dict[str, dict]) -> Tuple[set, set]:
    """Return referenced generator and grade sidecar paths."""
    result = _call("referenced-sidecars", domain_files=domain_files)
    return set(result.get("generators") or []), set(result.get("grades") or [])


def domain_file_map() -> Dict[str, str]:
    return {
        "cuts": "timeline/cuts.json",
        "color": "timeline/color.json",
        "audio": "timeline/audio.json",
        "effects": "timeline/effects.json",
        "markers": "timeline/markers.json",
        "metadata": "timeline/metadata.json",
        "manifest": "assets/manifest.json",
    }
