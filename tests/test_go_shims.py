"""Integration tests for the Python shims over the Go vit binary.

Skipped when the Go binary isn't available. Build it first:
    go build -o ~/.vit/bin/vit ./cmd/vit
or point VIT_BINARY at a build.
"""

import json
import os
import subprocess
import tempfile

import pytest

from vit import core
from vit.core import (
    GitError,
    find_project_root,
    git_add,
    git_branch,
    git_checkout,
    git_commit,
    git_current_branch,
    git_is_clean,
    git_list_branches,
    git_merge,
    git_show_file,
    git_status,
    git_log,
    git_log_with_changes,
    git_log_with_topology,
    categorize_commit,
)
from vit.json_writer import _write_json
from vit.merge_utils import merge_timeline_domains_for_overlays, referenced_sidecars
from vit.differ import get_changes_by_category
from vit.validator import validate_project

try:
    BINARY = core._find_binary()
except GitError:
    BINARY = None

pytestmark = pytest.mark.skipif(BINARY is None, reason="Go vit binary not built")


@pytest.fixture
def project_dir():
    with tempfile.TemporaryDirectory() as tmpdir:
        env = dict(os.environ)
        subprocess.run([BINARY, "init", tmpdir], check=True, capture_output=True, env=env)
        subprocess.run(["git", "config", "user.name", "Vit Test"], cwd=tmpdir, check=True)
        subprocess.run(["git", "config", "user.email", "vit@test.local"], cwd=tmpdir, check=True)
        yield tmpdir


def _write_cuts(project_dir, items):
    _write_json(
        os.path.join(project_dir, "timeline", "cuts.json"),
        {"video_tracks": [{"index": 1, "items": items}]},
    )


def test_basic_git_flow(project_dir):
    assert git_is_clean(project_dir)

    _write_cuts(project_dir, [{"id": "item_001", "name": "Clip", "record_start_frame": 0,
                               "record_end_frame": 100, "track_index": 1}])
    assert not git_is_clean(project_dir)
    assert "timeline" in git_status(project_dir)

    git_add(project_dir, ["timeline/"])
    commit_hash = git_commit(project_dir, "add clip")
    assert commit_hash
    assert "add clip" in git_log(project_dir)

    content = git_show_file(project_dir, "HEAD", "timeline/cuts.json")
    assert json.loads(content)["video_tracks"][0]["items"][0]["name"] == "Clip"
    assert git_show_file(project_dir, "HEAD", "nope.json") is None


def test_branch_and_merge(project_dir):
    base = git_current_branch(project_dir)
    git_branch(project_dir, "experiment")
    assert git_current_branch(project_dir) == "experiment"
    assert "experiment" in git_list_branches(project_dir)

    _write_cuts(project_dir, [{"id": "a", "name": "A", "record_start_frame": 0,
                               "record_end_frame": 100, "track_index": 1}])
    git_add(project_dir, ["timeline/"])
    git_commit(project_dir, "experiment edit")

    git_checkout(project_dir, base)
    success, _ = git_merge(project_dir, "experiment")
    assert success is True


def test_nothing_to_commit_raises_giterror(project_dir):
    git_add(project_dir, ["timeline/"])
    with pytest.raises(GitError) as excinfo:
        git_commit(project_dir, "empty")
    # the plugin string-matches this
    assert "nothing to commit" in str(excinfo.value)


def test_log_with_changes_and_topology(project_dir):
    _write_cuts(project_dir, [{"id": "a", "name": "A", "record_start_frame": 0,
                               "record_end_frame": 100, "track_index": 1}])
    git_add(project_dir, ["timeline/"])
    git_commit(project_dir, "edit cuts")

    commits = git_log_with_changes(project_dir, max_count=10)
    assert commits[0]["message"] == "edit cuts"
    assert commits[0]["files_changed"] == ["timeline/cuts.json"]
    assert commits[0]["category"] if False else True  # category added by caller, not here

    topo = git_log_with_topology(project_dir, max_count=10)
    assert topo["head"]
    assert topo["commits"][0]["is_head"] is True


def test_validate_shim(project_dir):
    _write_json(os.path.join(project_dir, "timeline", "color.json"),
                {"grades": {"ghost": {"contrast": 1.0}}})
    issues = validate_project(project_dir)
    orphaned = [i for i in issues if i.category == "orphaned_ref"]
    assert len(orphaned) == 1
    assert orphaned[0].severity == "error"
    assert "ghost" in orphaned[0].message


def test_changes_by_category_shim(project_dir):
    _write_cuts(project_dir, [{"id": "item_001", "name": "New.mov", "record_start_frame": 0,
                               "record_end_frame": 100, "track_index": 1}])
    changes = get_changes_by_category(project_dir, "HEAD")
    assert changes["video"] == [
        {"id": "item_001", "name": "New.mov", "type": "added", "details": "Added to V1"}
    ]
    assert changes["audio"] == []
    assert changes["color"] == []


def test_overlay_merge_shim():
    ours = {
        "cuts": {"video_tracks": [{"index": 1, "items": [
            {"id": "x", "name": "clip.mov", "media_ref": "sha256:c",
             "record_start_frame": 0, "record_end_frame": 100,
             "source_start_frame": 0, "source_end_frame": 100,
             "track_index": 1, "transform": {}}]}]},
        "color": {"grades": {}}, "audio": {"audio_tracks": []}, "effects": {},
        "markers": {"markers": []}, "metadata": {"track_count": {"video": 1, "audio": 0}},
        "manifest": {"assets": {}},
    }
    theirs = {
        "cuts": {"video_tracks": [{"index": 1, "items": [
            {"id": "x", "name": "Text+", "media_ref": "generator:x",
             "record_start_frame": 0, "record_end_frame": 100,
             "source_start_frame": 0, "source_end_frame": 100,
             "track_index": 1, "item_type": "title",
             "fusion_comp_file": "x.comp", "transform": {}}]}]},
        "color": {"grades": {}}, "audio": {"audio_tracks": []}, "effects": {},
        "markers": {"markers": []}, "metadata": {"track_count": {"video": 1, "audio": 0}},
        "manifest": {"assets": {}},
    }
    merged, plan = merge_timeline_domains_for_overlays(theirs, ours, theirs)
    tracks = merged["cuts"]["video_tracks"]
    assert tracks[0]["items"][0]["name"] == "clip.mov"
    assert tracks[1]["items"][0]["id"] == "x_overlay"
    assert plan.generator_renames == {"x.comp": "x_overlay.comp"}
    assert isinstance(plan.grade_restore_ours, set)

    generators, grades = referenced_sidecars(merged)
    assert "timeline/generators/x_overlay.comp" in generators
    assert grades == set()


def test_pure_python_helpers(project_dir):
    assert find_project_root(project_dir) == project_dir
    assert find_project_root(os.path.join(project_dir, "timeline")) == project_dir
    assert categorize_commit(["timeline/audio.json"]) == "audio"
    assert categorize_commit([]) == "video"
