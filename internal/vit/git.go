// Git wrapper — all git operations go through the system git binary.
// Direct port of vit/core.py.
package vit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitError is raised when a git command fails (parity with core.GitError —
// the Resolve plugin string-matches messages like "nothing to commit").
type GitError struct{ Msg string }

func (e *GitError) Error() string { return e.Msg }

type runResult struct {
	stdout string
	stderr string
	code   int
}

func run(cwd string, args ...string) (runResult, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	res := runResult{stdout: out.String(), stderr: errb.String()}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.code = exitErr.ExitCode()
			return res, nil
		}
		return res, &GitError{Msg: fmt.Sprintf("git %s failed: %v", strings.Join(args, " "), err)}
	}
	return res, nil
}

func runChecked(cwd string, args ...string) (runResult, error) {
	res, err := run(cwd, args...)
	if err != nil {
		return res, err
	}
	if res.code != 0 {
		detail := strings.TrimSpace(res.stderr)
		if detail == "" {
			detail = strings.TrimSpace(res.stdout)
		}
		return res, &GitError{Msg: fmt.Sprintf("git %s failed: %s", strings.Join(args, " "), detail)}
	}
	return res, nil
}

const projectGitignore = `# OS files
.DS_Store
Thumbs.db
Desktop.ini

# Media files — vit tracks metadata, not binaries
*.mov
*.mp4
*.mxf
*.avi
*.mkv
*.wav
*.aif
*.aiff
*.mp3
*.aac
*.braw
*.r3d
*.arw
*.dng

# Render output
Render/
Deliver/

# DaVinci Resolve project files (managed by Resolve, not vit)
*.drp

# Environment / secrets
.env
.env.*

# Python
__pycache__/
*.pyc
`

func writeVitConfig(vitDir string) error {
	if err := os.MkdirAll(vitDir, 0o755); err != nil {
		return err
	}
	s, err := EncodePy(map[string]any{"version": "0.1.0", "nle": "resolve"})
	if err != nil {
		return err
	}
	// core.py writes config.json without a trailing newline
	return os.WriteFile(filepath.Join(vitDir, "config.json"), []byte(s), 0o644)
}

// GitInit initializes a new git repo and creates .vit/, timeline/, assets/.
func GitInit(projectDir string) error {
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	if _, err := runChecked(projectDir, "init"); err != nil {
		return err
	}
	if err := writeVitConfig(filepath.Join(projectDir, ".vit")); err != nil {
		return err
	}
	for _, d := range []string{"timeline", "assets"} {
		if err := os.MkdirAll(filepath.Join(projectDir, d), 0o755); err != nil {
			return err
		}
	}
	gitignore := filepath.Join(projectDir, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		if err := os.WriteFile(gitignore, []byte(projectGitignore), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func GitAdd(projectDir string, paths []string) error {
	_, err := runChecked(projectDir, append([]string{"add"}, paths...)...)
	return err
}

// GitCommit creates a commit and returns the short hash.
func GitCommit(projectDir, message string) (string, error) {
	res, err := runChecked(projectDir, "commit", "-m", message)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return strings.TrimRight(parts[1], "]"), nil
			}
		}
	}
	return "", nil
}

// GitBranch creates and switches to a new branch.
func GitBranch(projectDir, branchName string) error {
	_, err := runChecked(projectDir, "checkout", "-b", branchName)
	return err
}

func GitCheckout(projectDir, ref string) error {
	_, err := runChecked(projectDir, "checkout", ref)
	return err
}

// GitMerge attempts a merge. Returns (success, combined output).
func GitMerge(projectDir, branch string) (bool, string, error) {
	res, err := run(projectDir, "merge", branch)
	if err != nil {
		return false, "", err
	}
	return res.code == 0, res.stdout + res.stderr, nil
}

func GitMergeAbort(projectDir string) error {
	_, err := runChecked(projectDir, "merge", "--abort")
	return err
}

func GitDiff(projectDir, ref string) (string, error) {
	args := []string{"diff"}
	if ref != "" {
		args = append(args, ref)
	}
	res, err := runChecked(projectDir, args...)
	if err != nil {
		return "", err
	}
	return res.stdout, nil
}

func GitLog(projectDir string, maxCount int) (string, error) {
	res, err := runChecked(projectDir, "log", fmt.Sprintf("--max-count=%d", maxCount), "--oneline", "--decorate")
	if err != nil {
		return "", err
	}
	return res.stdout, nil
}

func GitStatus(projectDir string) (string, error) {
	res, err := runChecked(projectDir, "status", "--short")
	if err != nil {
		return "", err
	}
	return res.stdout, nil
}

func GitRevert(projectDir string) error {
	_, err := runChecked(projectDir, "revert", "HEAD", "--no-edit")
	return err
}

func GitPush(projectDir, remote, branch string) (string, error) {
	args := []string{"push", remote}
	if branch != "" {
		args = append(args, branch)
	}
	res, err := runChecked(projectDir, args...)
	if err != nil {
		return "", err
	}
	return res.stdout + res.stderr, nil
}

func GitPull(projectDir, remote, branch string) (string, error) {
	args := []string{"pull", remote}
	if branch != "" {
		args = append(args, branch)
	}
	res, err := runChecked(projectDir, args...)
	if err != nil {
		return "", err
	}
	return res.stdout + res.stderr, nil
}

func GitCurrentBranch(projectDir string) (string, error) {
	res, err := runChecked(projectDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.stdout), nil
}

func GitListBranches(projectDir string) ([]string, error) {
	res, err := runChecked(projectDir, "branch", "--list")
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for _, line := range strings.Split(res.stdout, "\n") {
		branch := strings.TrimLeft(strings.TrimSpace(line), "* ")
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// GitShowFile returns file content at a ref, or ok=false if unavailable.
func GitShowFile(projectDir, ref, filepath_ string) (string, bool, error) {
	res, err := run(projectDir, "show", ref+":"+filepath_)
	if err != nil {
		return "", false, err
	}
	if res.code != 0 {
		return "", false, nil
	}
	return res.stdout, true, nil
}

// GitShowFileRaw returns raw bytes of a file at a ref (for binary sidecars).
func GitShowFileRaw(projectDir, ref, filepath_ string) ([]byte, bool, error) {
	cmd := exec.Command("git", "show", ref+":"+filepath_)
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, false, nil
		}
		return nil, false, err
	}
	return out, true, nil
}

func GitMergeBase(projectDir, ref1, ref2 string) (string, bool, error) {
	res, err := run(projectDir, "merge-base", ref1, ref2)
	if err != nil {
		return "", false, err
	}
	if res.code != 0 {
		return "", false, nil
	}
	return strings.TrimSpace(res.stdout), true, nil
}

func GitListConflictedFiles(projectDir string) ([]string, error) {
	res, err := run(projectDir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, f := range strings.Split(res.stdout, "\n") {
		if strings.TrimSpace(f) != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// GitCheckoutTheirs resolves conflicts by taking the incoming branch's version.
func GitCheckoutTheirs(projectDir string, paths []string) error {
	_, err := runChecked(projectDir, append([]string{"checkout", "--theirs"}, paths...)...)
	return err
}

func GitIsClean(projectDir string) (bool, error) {
	res, err := run(projectDir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.stdout) == "", nil
}

func IsGitRepo(projectDir string) bool {
	info, err := os.Stat(projectDir)
	if err != nil || !info.IsDir() {
		return false
	}
	res, err := run(projectDir, "rev-parse", "--git-dir")
	return err == nil && res.code == 0
}

// FindProjectRoot walks up from startDir looking for a .vit/ directory.
func FindProjectRoot(startDir string) (string, bool) {
	current := startDir
	if current == "" {
		var err error
		current, err = os.Getwd()
		if err != nil {
			return "", false
		}
	}
	for {
		if info, err := os.Stat(filepath.Join(current, ".vit")); err == nil && info.IsDir() {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func GitRemoteAdd(projectDir, name, url string) error {
	_, err := runChecked(projectDir, "remote", "add", name, url)
	return err
}

// GitRemoteList returns remotes as {name, url} pairs (fetch URLs).
func GitRemoteList(projectDir string) ([]map[string]string, error) {
	res, err := run(projectDir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	remotes := []map[string]string{}
	for _, line := range strings.Split(res.stdout, "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.Contains(line, "(fetch)") && !seen[parts[0]] {
			seen[parts[0]] = true
			remotes = append(remotes, map[string]string{"name": parts[0], "url": parts[1]})
		}
	}
	return remotes, nil
}

func GitRemoteRemove(projectDir, name string) error {
	_, err := runChecked(projectDir, "remote", "remove", name)
	return err
}

// GitClone clones a remote vit repo and ensures .vit/config.json exists.
func GitClone(url, destDir string) error {
	cmd := exec.Command("git", "clone", url, destDir)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(errb.String())
		if detail == "" {
			detail = strings.TrimSpace(out.String())
		}
		return &GitError{Msg: fmt.Sprintf("git clone failed: %s", detail)}
	}
	configPath := filepath.Join(destDir, ".vit", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return writeVitConfig(filepath.Join(destDir, ".vit"))
	}
	return nil
}

func GitConfigGet(projectDir, key string) (string, error) {
	res, err := run(projectDir, "config", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.stdout), nil
}

func GitConfigSet(projectDir, key, value string) error {
	_, err := runChecked(projectDir, "config", key, value)
	return err
}

func GitPushSetUpstream(projectDir, remote, branch string) (string, error) {
	if branch == "" {
		var err error
		branch, err = GitCurrentBranch(projectDir)
		if err != nil {
			return "", err
		}
	}
	res, err := runChecked(projectDir, "push", "-u", remote, branch)
	if err != nil {
		return "", err
	}
	return res.stdout + res.stderr, nil
}

// GitLogWithChanges returns commits with per-commit file lists
// (hash, message, branch, date, files_changed).
func GitLogWithChanges(projectDir string, maxCount int) ([]map[string]any, error) {
	res, err := run(projectDir,
		"log",
		fmt.Sprintf("--max-count=%d", maxCount),
		"--pretty=format:%H|%s|%ad|%D",
		"--date=relative",
		"--name-only",
	)
	if err != nil {
		return nil, err
	}
	if res.code != 0 {
		return []map[string]any{}, nil
	}

	commits := []map[string]any{}
	var current map[string]any
	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if strings.Count(line, "|") >= 3 {
			if current != nil {
				commits = append(commits, current)
			}
			parts := strings.SplitN(line, "|", 4)
			refs := ""
			if len(parts) > 3 {
				refs = parts[3]
			}
			branch := "main"
			if refs != "" {
				for _, ref := range strings.Split(refs, ",") {
					ref = strings.TrimSpace(ref)
					if strings.HasPrefix(ref, "HEAD -> ") {
						branch = strings.TrimPrefix(ref, "HEAD -> ")
						break
					} else if !strings.Contains(ref, "/") && ref != "HEAD" && ref != "" {
						branch = ref
						break
					}
				}
			}
			hash := parts[0]
			if len(hash) > 7 {
				hash = hash[:7]
			}
			current = map[string]any{
				"hash":          hash,
				"message":       parts[1],
				"date":          parts[2],
				"branch":        branch,
				"files_changed": []any{},
			}
		} else if strings.TrimSpace(line) != "" && current != nil {
			current["files_changed"] = append(current["files_changed"].([]any), strings.TrimSpace(line))
		}
	}
	if current != nil {
		commits = append(commits, current)
	}
	return commits, nil
}

// CategorizeCommit picks the dominant category ("audio", "video", "color")
// for a commit from its changed file paths.
func CategorizeCommit(filesChanged []string) string {
	counts := map[string]int{"audio": 0, "video": 0, "color": 0}
	for _, f := range filesChanged {
		lower := strings.ToLower(f)
		switch {
		case strings.Contains(lower, "audio"):
			counts["audio"]++
		case strings.Contains(lower, "color"):
			counts["color"]++
		case strings.Contains(lower, "cuts"), strings.Contains(lower, "video"):
			counts["video"]++
		}
	}
	maxCat, maxN := "video", counts["video"]
	for _, cat := range []string{"audio", "color"} {
		if counts[cat] > maxN {
			maxCat, maxN = cat, counts[cat]
		}
	}
	if maxN == 0 {
		return "video"
	}
	return maxCat
}

// GitLogWithTopology returns commits with parent hashes for graph rendering.
func GitLogWithTopology(projectDir string, maxCount int) (map[string]any, error) {
	empty := map[string]any{"commits": []any{}, "branches": []any{}, "head": ""}

	headHash := ""
	if res, err := run(projectDir, "rev-parse", "HEAD"); err == nil && res.code == 0 {
		headHash = strings.TrimSpace(res.stdout)
		if len(headHash) > 7 {
			headHash = headHash[:7]
		}
	}

	currentBranch := "main"
	if res, err := run(projectDir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && res.code == 0 {
		currentBranch = strings.TrimSpace(res.stdout)
	}

	mainBranch := "main"
	for _, candidate := range []string{"main", "master"} {
		if res, err := run(projectDir, "rev-parse", "--verify", candidate); err == nil && res.code == 0 {
			mainBranch = candidate
			break
		}
	}

	mainCommits := map[string]bool{}
	mainCmd := []string{"log", mainBranch, "--pretty=format:%H"}
	if maxCount > 0 {
		mainCmd = []string{"log", mainBranch, fmt.Sprintf("--max-count=%d", maxCount), "--pretty=format:%H"}
	}
	if res, err := run(projectDir, mainCmd...); err == nil && res.code == 0 {
		for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				if len(line) > 7 {
					line = line[:7]
				}
				mainCommits[line] = true
			}
		}
	}

	logCmd := []string{"log", "--all", "--pretty=format:%H|%P|%s|%D", "--date-order"}
	if maxCount > 0 {
		logCmd = []string{"log", "--all", fmt.Sprintf("--max-count=%d", maxCount), "--pretty=format:%H|%P|%s|%D", "--date-order"}
	}
	res, err := run(projectDir, logCmd...)
	if err != nil || res.code != 0 {
		return empty, err
	}

	commits := []any{}
	branchSet := map[string]bool{}
	branchList := []any{}

	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 3 {
			continue
		}
		hashShort := parts[0]
		if len(hashShort) > 7 {
			hashShort = hashShort[:7]
		}
		refs := ""
		if len(parts) > 3 {
			refs = parts[3]
		}

		parents := []any{}
		for _, p := range strings.Fields(parts[1]) {
			if len(p) > 7 {
				p = p[:7]
			}
			parents = append(parents, p)
		}

		isMainCommit := mainCommits[hashShort]

		branch := ""
		isHead := false
		if refs != "" {
			for _, ref := range strings.Split(refs, ",") {
				ref = strings.TrimSpace(ref)
				if strings.HasPrefix(ref, "HEAD -> ") {
					branch = strings.TrimPrefix(ref, "HEAD -> ")
					isHead = true
					break
				} else if ref == "HEAD" {
					isHead = true
				} else if !strings.Contains(ref, "/") && ref != "HEAD" && ref != "" {
					if branch == "" {
						branch = ref
					}
				}
			}
		}
		if branch == "" {
			if isMainCommit {
				branch = mainBranch
			} else {
				branch = currentBranch
			}
		}
		if !branchSet[branch] {
			branchSet[branch] = true
			branchList = append(branchList, branch)
		}

		commits = append(commits, map[string]any{
			"hash":           hashShort,
			"parents":        parents,
			"message":        parts[2],
			"branch":         branch,
			"is_head":        isHead || hashShort == headHash,
			"is_main_commit": isMainCommit,
		})
	}

	return map[string]any{
		"commits":  commits,
		"branches": branchList,
		"head":     headHash,
	}, nil
}
