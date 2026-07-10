package vit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newProject creates a temp dir with an initialized vit project and initial
// commit (parity with tests/test_core.py fixture).
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := GitInit(dir); err != nil {
		t.Fatal(err)
	}
	// tests must not depend on global git identity
	for _, kv := range [][2]string{{"user.name", "Vit Test"}, {"user.email", "vit@test.local"}} {
		if err := GitConfigSet(dir, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := GitAdd(dir, []string{".vit/", "timeline/", "assets/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "initial commit"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeCuts(t *testing.T, dir string, items []any) {
	t.Helper()
	data := map[string]any{"video_tracks": []any{map[string]any{"index": 1, "items": items}}}
	if err := WriteJSONFile(filepath.Join(dir, "timeline", "cuts.json"), data); err != nil {
		t.Fatal(err)
	}
}

func TestInitCreatesStructure(t *testing.T) {
	dir := newProject(t)
	for _, sub := range []string{".vit", "timeline", "assets"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			t.Errorf("missing directory %s", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".vit", "config.json")); err != nil {
		t.Error("missing .vit/config.json")
	}
}

func TestIsGitRepo(t *testing.T) {
	dir := newProject(t)
	if !IsGitRepo(dir) {
		t.Error("expected git repo")
	}
	if IsGitRepo("/tmp/nonexistent_dir_xyz") {
		t.Error("expected not a git repo")
	}
}

func TestCommitAndLog(t *testing.T) {
	dir := newProject(t)
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "Test Clip"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "add test clip"); err != nil {
		t.Fatal(err)
	}
	log, err := GitLog(dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "add test clip") {
		t.Errorf("log missing commit: %s", log)
	}
}

func TestBranchAndCheckout(t *testing.T) {
	dir := newProject(t)
	if err := GitBranch(dir, "color-grade"); err != nil {
		t.Fatal(err)
	}
	if b, _ := GitCurrentBranch(dir); b != "color-grade" {
		t.Errorf("expected color-grade, got %s", b)
	}
	if err := GitCheckout(dir, defaultBranch(t, dir)); err != nil {
		t.Fatal(err)
	}
	branches, err := GitListBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(branches, ",")
	if !strings.Contains(joined, "color-grade") {
		t.Errorf("branches missing color-grade: %v", branches)
	}
}

// defaultBranch returns the repo's initial branch name (main or master
// depending on git config).
func defaultBranch(t *testing.T, dir string) string {
	t.Helper()
	branches, err := GitListBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		if b == "main" || b == "master" {
			return b
		}
	}
	return branches[0]
}

func TestMergeClean(t *testing.T) {
	dir := newProject(t)
	base := defaultBranch(t, dir)

	if err := GitBranch(dir, "color-grade"); err != nil {
		t.Fatal(err)
	}
	colorData := map[string]any{"grades": map[string]any{"item_001": map[string]any{
		"num_nodes": 2,
		"nodes": []any{
			map[string]any{"index": 1, "label": "", "lut": ""},
			map[string]any{"index": 2, "label": "LUT", "lut": "Warm.cube"},
		},
		"version_name": "", "drx_file": nil,
	}}}
	if err := WriteJSONFile(filepath.Join(dir, "timeline", "color.json"), colorData); err != nil {
		t.Fatal(err)
	}
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "add color grade"); err != nil {
		t.Fatal(err)
	}

	if err := GitCheckout(dir, base); err != nil {
		t.Fatal(err)
	}
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "Updated Clip"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "update clip name"); err != nil {
		t.Fatal(err)
	}

	success, _, err := GitMerge(dir, "color-grade")
	if err != nil {
		t.Fatal(err)
	}
	if !success {
		t.Error("expected clean merge")
	}
}

func TestMergeConflict(t *testing.T) {
	dir := newProject(t)
	base := defaultBranch(t, dir)

	if err := GitBranch(dir, "experiment"); err != nil {
		t.Fatal(err)
	}
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "Experiment Clip"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "experiment edit"); err != nil {
		t.Fatal(err)
	}

	if err := GitCheckout(dir, base); err != nil {
		t.Fatal(err)
	}
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "Main Clip"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "main edit"); err != nil {
		t.Fatal(err)
	}

	success, _, err := GitMerge(dir, "experiment")
	if err != nil {
		t.Fatal(err)
	}
	if success {
		t.Error("expected merge conflict")
	}
	conflicted, err := GitListConflictedFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicted) == 0 {
		t.Error("expected conflicted files")
	}
}

func TestGitShowFile(t *testing.T) {
	dir := newProject(t)
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "V1"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "version 1"); err != nil {
		t.Fatal(err)
	}
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "V2"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "version 2"); err != nil {
		t.Fatal(err)
	}

	assertName := func(ref, want string) {
		content, ok, err := GitShowFile(dir, ref, "timeline/cuts.json")
		if err != nil || !ok {
			t.Fatalf("show %s failed: %v", ref, err)
		}
		v, err := DecodeJSON([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
		m := v.(map[string]any)
		tracks := m["video_tracks"].([]any)
		items := tracks[0].(map[string]any)["items"].([]any)
		name := items[0].(map[string]any)["name"].(string)
		if name != want {
			t.Errorf("ref %s: expected %s, got %s", ref, want, name)
		}
	}
	assertName("HEAD", "V2")
	assertName("HEAD~1", "V1")
}

func TestGitStatusShowsModified(t *testing.T) {
	dir := newProject(t)
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "New"}})
	status, err := GitStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "timeline") {
		t.Errorf("status missing timeline: %q", status)
	}
}

func TestFindProjectRoot(t *testing.T) {
	dir := newProject(t)
	// macOS: t.TempDir may be a symlinked path; resolve like the walker sees it
	if found, ok := FindProjectRoot(dir); !ok || found != dir {
		t.Errorf("expected %s, got %s", dir, found)
	}
	sub := filepath.Join(dir, "timeline")
	if found, ok := FindProjectRoot(sub); !ok || found != dir {
		t.Errorf("from subdir: expected %s, got %s", dir, found)
	}
}

func TestRevert(t *testing.T) {
	dir := newProject(t)
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "Before"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "before revert"); err != nil {
		t.Fatal(err)
	}
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "After"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "after revert"); err != nil {
		t.Fatal(err)
	}
	if err := GitRevert(dir); err != nil {
		t.Fatal(err)
	}
	log, err := GitLog(dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "Revert") {
		t.Errorf("log missing revert: %s", log)
	}
}

func TestCommitNothingToCommitError(t *testing.T) {
	dir := newProject(t)
	// real flows stage everything (incl. .gitignore) before committing
	if err := GitAdd(dir, []string{".gitignore"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "add gitignore"); err != nil {
		t.Fatal(err)
	}
	_, err := GitCommit(dir, "empty")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nothing to commit") {
		t.Errorf("plugin depends on 'nothing to commit' in error, got: %s", err.Error())
	}
}

func TestCategorizeCommit(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{[]string{"timeline/audio.json"}, "audio"},
		{[]string{"timeline/color.json"}, "color"},
		{[]string{"timeline/cuts.json"}, "video"},
		{[]string{"timeline/markers.json"}, "video"},
		{[]string{"timeline/audio.json", "timeline/audio2.json", "timeline/cuts.json"}, "audio"},
		{nil, "video"},
	}
	for _, c := range cases {
		if got := CategorizeCommit(c.files); got != c.want {
			t.Errorf("CategorizeCommit(%v) = %s, want %s", c.files, got, c.want)
		}
	}
}

func TestGitLogWithChangesAndTopology(t *testing.T) {
	dir := newProject(t)
	writeCuts(t, dir, []any{map[string]any{"id": "item_001", "name": "clip"}})
	if err := GitAdd(dir, []string{"timeline/"}); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommit(dir, "edit cuts"); err != nil {
		t.Fatal(err)
	}

	commits, err := GitLogWithChanges(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0]["message"] != "edit cuts" {
		t.Errorf("unexpected message: %v", commits[0]["message"])
	}
	files := commits[0]["files_changed"].([]any)
	if len(files) != 1 || files[0] != "timeline/cuts.json" {
		t.Errorf("unexpected files: %v", files)
	}

	topo, err := GitLogWithTopology(dir, 30)
	if err != nil {
		t.Fatal(err)
	}
	topoCommits := topo["commits"].([]any)
	if len(topoCommits) != 2 {
		t.Fatalf("expected 2 topo commits, got %d", len(topoCommits))
	}
	head := topoCommits[0].(map[string]any)
	if head["is_head"] != true {
		t.Error("first commit should be HEAD")
	}
	if topo["head"] == "" {
		t.Error("missing head hash")
	}
}
