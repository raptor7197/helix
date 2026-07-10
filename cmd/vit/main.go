// Vit CLI — all user interaction goes through here.
// Direct port of vit/cli.py.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	vit "github.com/raptor7197/vit/internal/vit"
)

const version = "0.1.0"

var stdin = bufio.NewReader(os.Stdin)

func input(prompt string) string {
	fmt.Print(prompt)
	line, err := stdin.ReadString('\n')
	if err != nil {
		fmt.Println()
		return ""
	}
	return strings.TrimSpace(line)
}

// parseArgs separates flags from positionals (argparse allows them
// intermixed). aliases maps every accepted form to a canonical name;
// valueFlags lists canonical names that consume a value.
func parseArgs(args []string, aliases map[string]string, valueFlags map[string]bool) (map[string]string, []string, error) {
	flags := map[string]string{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, value := arg, ""
		hasInline := false
		if idx := strings.Index(arg, "="); idx > 0 && strings.HasPrefix(arg, "-") {
			name, value = arg[:idx], arg[idx+1:]
			hasInline = true
		}
		canonical, isFlag := aliases[name]
		if !isFlag {
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return nil, nil, fmt.Errorf("unrecognized argument: %s", arg)
			}
			pos = append(pos, arg)
			continue
		}
		if valueFlags[canonical] {
			if !hasInline {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("flag %s requires a value", name)
				}
				i++
				value = args[i]
			}
			flags[canonical] = value
		} else {
			flags[canonical] = "true"
		}
	}
	return flags, pos, nil
}

func requireProject() string {
	root, ok := vit.FindProjectRoot("")
	if !ok {
		fmt.Println("Error: Not a vit project. Run 'vit init' first.")
		os.Exit(1)
	}
	return root
}

func exitOnGitError(err error) {
	if err != nil {
		if _, ok := err.(*vit.GitError); ok {
			fmt.Printf("Error: %s\n", err.Error())
			os.Exit(1)
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// emptyTimelineFiles are the initial domain files written by `vit init`
// (parity with json_writer.write_timeline(Timeline())).
func writeEmptyTimeline(projectDir string) error {
	files := map[string]any{
		"timeline/cuts.json":     map[string]any{"video_tracks": []any{}},
		"timeline/color.json":    map[string]any{"grades": map[string]any{}},
		"timeline/audio.json":    map[string]any{"audio_tracks": []any{}},
		"timeline/effects.json":  map[string]any{},
		"timeline/markers.json":  map[string]any{"markers": []any{}},
		"timeline/metadata.json": emptyMetadata(),
		"assets/manifest.json":   map[string]any{"assets": map[string]any{}},
	}
	for rel, data := range files {
		if err := vit.WriteJSONFile(filepath.Join(projectDir, rel), data); err != nil {
			return err
		}
	}
	return nil
}

func emptyMetadata() map[string]any {
	return map[string]any{
		"project_name":   "",
		"timeline_name":  "",
		"frame_rate":     jsonNumber("24.0"),
		"resolution":     map[string]any{"width": 1920, "height": 1080},
		"start_timecode": "01:00:00:00",
		"track_count":    map[string]any{"video": 1, "audio": 1},
	}
}

func cmdInit(args []string) {
	_, pos, err := parseArgs(args, map[string]string{}, map[string]bool{})
	exitOnGitError(err)
	projectDir, _ := os.Getwd()
	if len(pos) > 0 {
		projectDir = pos[0]
	}

	if info, err := os.Stat(filepath.Join(projectDir, ".vit")); err == nil && info.IsDir() {
		fmt.Printf("Error: '%s' is already a vit project.\n", projectDir)
		os.Exit(1)
	}

	exitOnGitError(vit.GitInit(projectDir))
	exitOnGitError(writeEmptyTimeline(projectDir))
	exitOnGitError(vit.GitAdd(projectDir, []string{".vit/", "timeline/", "assets/", ".gitignore"}))
	_, err = vit.GitCommit(projectDir, "vit: initial snapshot")
	exitOnGitError(err)
	fmt.Printf("  Initialized vit project in %s\n", projectDir)
	fmt.Println("  Created: .vit/, timeline/, assets/")
	fmt.Println("  Initial snapshot committed.")
}

func cmdAdd(args []string) {
	projectDir := requireProject()
	// The serializer is called from Resolve plugin scripts, not from the CLI.
	// CLI 'add' just stages the JSON files that were already written.
	exitOnGitError(vit.GitAdd(projectDir, []string{"timeline/", "assets/"}))
	fmt.Println("  Staged timeline and asset files.")
}

func ensureGitIdentity(projectDir string) {
	name, _ := vit.GitConfigGet(projectDir, "user.name")
	email, _ := vit.GitConfigGet(projectDir, "user.email")
	if name == "" || email == "" {
		fmt.Println("  Git identity not set. This ensures commits show who made each change.")
		if name == "" {
			name = input("  Your name: ")
			if name != "" {
				_ = vit.GitConfigSet(projectDir, "user.name", name)
			}
		}
		if email == "" {
			email = input("  Your email: ")
			if email != "" {
				_ = vit.GitConfigSet(projectDir, "user.email", email)
			}
		}
	}
}

func cmdCommit(args []string) {
	flags, _, err := parseArgs(args,
		map[string]string{"-m": "message", "--message": "message"},
		map[string]bool{"message": true})
	exitOnGitError(err)

	projectDir := requireProject()
	ensureGitIdentity(projectDir)
	exitOnGitError(vit.GitAdd(projectDir, []string{"timeline/", "assets/", ".vit/", ".gitignore"}))

	message := flags["message"]
	if message == "" {
		if diffText, diffErr := vit.DiffFromProject(projectDir, "HEAD"); diffErr == nil && strings.TrimSpace(diffText) != "" {
			if suggestion := vit.SuggestCommitMessage(diffText); suggestion != "" {
				fmt.Printf("  AI suggested: \"%s\"\n", suggestion)
				response := strings.ToLower(input("  Use this message? [Y/n/edit]: "))
				switch response {
				case "", "y", "yes":
					message = suggestion
				case "n", "no":
					// fall through to default
				default:
					message = response
				}
			}
		}
		if message == "" {
			message = "vit: save version"
		}
	}

	commitHash, err := vit.GitCommit(projectDir, message)
	if err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			fmt.Println("  Nothing to commit — timeline unchanged.")
			return
		}
		exitOnGitError(err)
	}
	fmt.Printf("  Committed: %s\n", message)
	if commitHash != "" {
		fmt.Printf("  Hash: %s\n", commitHash)
	}
}

func cmdBranch(args []string) {
	flags, pos, err := parseArgs(args,
		map[string]string{"-l": "list", "--list": "list"},
		map[string]bool{})
	exitOnGitError(err)
	projectDir := requireProject()

	if flags["list"] == "true" {
		branches, err := vit.GitListBranches(projectDir)
		exitOnGitError(err)
		current, err := vit.GitCurrentBranch(projectDir)
		exitOnGitError(err)
		for _, b := range branches {
			prefix := "  "
			if b == current {
				prefix = "* "
			}
			fmt.Printf("%s%s\n", prefix, b)
		}
		return
	}

	if len(pos) == 0 {
		current, err := vit.GitCurrentBranch(projectDir)
		exitOnGitError(err)
		fmt.Printf("  Current branch: %s\n", current)
		return
	}

	exitOnGitError(vit.GitBranch(projectDir, pos[0]))
	fmt.Printf("  Created and switched to branch '%s'\n", pos[0])
}

func cmdCheckout(args []string) {
	_, pos, err := parseArgs(args, map[string]string{}, map[string]bool{})
	exitOnGitError(err)
	if len(pos) == 0 {
		fmt.Println("Error: checkout requires a branch name or commit hash.")
		os.Exit(1)
	}
	projectDir := requireProject()
	exitOnGitError(vit.GitCheckout(projectDir, pos[0]))
	fmt.Printf("  Switched to '%s'\n", pos[0])
}

func loadFilesAtRef(projectDir, ref string) map[string]any {
	files := map[string]any{}
	for _, domain := range vit.DomainOrder {
		content, ok, _ := vit.GitShowFile(projectDir, ref, vit.DomainFileMap()[domain])
		files[domain] = map[string]any{}
		if ok {
			if v, err := vit.DecodeJSON([]byte(content)); err == nil {
				if m, isMap := v.(map[string]any); isMap {
					files[domain] = m
				}
			}
		}
	}
	return files
}

func cmdMerge(args []string) {
	flags, pos, err := parseArgs(args,
		map[string]string{"--no-ai": "no_ai"},
		map[string]bool{})
	exitOnGitError(err)
	if len(pos) == 0 {
		fmt.Println("Error: merge requires a branch name.")
		os.Exit(1)
	}
	branch := pos[0]
	noAI := flags["no_ai"] == "true"

	projectDir := requireProject()
	current, err := vit.GitCurrentBranch(projectDir)
	exitOnGitError(err)

	// Auto-commit any outstanding changes before merge
	clean, err := vit.GitIsClean(projectDir)
	exitOnGitError(err)
	if !clean {
		fmt.Println("  Auto-saving uncommitted changes before merge...")
		exitOnGitError(vit.GitAdd(projectDir, []string{"timeline/", "assets/", ".vit/", ".gitignore"}))
		if _, err := vit.GitCommit(projectDir, fmt.Sprintf("vit: auto-save before merging '%s'", branch)); err != nil {
			if !strings.Contains(err.Error(), "nothing to commit") {
				exitOnGitError(err)
			}
		}
	}

	// Pre-merge AI analysis
	if !noAI {
		changesOurs, changesTheirs, diffErr := vit.GetBranchDiffByCategory(projectDir, current, branch)
		if diffErr == nil {
			hasChanges := false
			for _, c := range []map[string][]map[string]any{changesOurs, changesTheirs} {
				for _, items := range c {
					if len(items) > 0 {
						hasChanges = true
					}
				}
			}
			if hasChanges {
				fmt.Printf("  Analyzing merge of '%s' into '%s'...\n", branch, current)
				analysis := vit.AnalyzeBranchComparison(current, branch, changesOurs, changesTheirs)
				rec := stringOr(analysis["recommendation"], "manual_review")
				explanation := stringOr(analysis["explanation"], "")
				conflicts := []string{}
				if c, ok := analysis["conflicts"].([]any); ok {
					for _, v := range c {
						if s, isStr := v.(string); isStr {
							conflicts = append(conflicts, s)
						}
					}
				}
				if len(conflicts) > 0 {
					fmt.Printf("  ⚠ Potential conflicts: %s\n", strings.Join(conflicts, ", "))
				}
				if explanation != "" {
					fmt.Printf("  Analysis: %s\n", explanation)
				}
				if rec == "manual_review" {
					response := strings.ToLower(input("  Proceed with merge? [Y/n]: "))
					if response == "n" || response == "no" {
						fmt.Println("  Merge cancelled.")
						return
					}
				}
			}
		}
	}

	fmt.Printf("  Merging '%s' into '%s'...\n", branch, current)

	preMergeFiles, err := vit.ReadAllDomainFiles(projectDir)
	exitOnGitError(err)

	success, _, err := vit.GitMerge(projectDir, branch)
	exitOnGitError(err)

	if success {
		fmt.Println("  Git merge succeeded.")
		issues, err := vit.ValidateProject(projectDir)
		exitOnGitError(err)

		mergeBaseRef, hasBase, _ := vit.GitMergeBase(projectDir, current, branch)
		baseFiles := map[string]any{}
		if hasBase {
			baseFiles = loadFilesAtRef(projectDir, mergeBaseRef)
		}
		theirsFiles := loadFilesAtRef(projectDir, branch)

		overlappingDomains := vit.DetectOverlappingDomains(baseFiles, preMergeFiles, theirsFiles)

		hasErrors := false
		for _, i := range issues {
			if i.Severity == "error" {
				hasErrors = true
			}
		}
		needsAIReview := !noAI && (hasErrors || len(overlappingDomains) > 0)

		if len(issues) == 0 && len(overlappingDomains) == 0 {
			fmt.Println("  Post-merge validation passed.")
			return
		}
		if len(issues) > 0 {
			fmt.Println("\n  Post-merge validation found issues:")
			fmt.Println(vit.FormatIssues(issues))
		}
		if len(overlappingDomains) > 0 && len(issues) == 0 {
			fmt.Printf("\n  Both branches modified: %s\n", strings.Join(overlappingDomains, ", "))
			fmt.Println("  Running AI review to check for semantic conflicts...")
		}

		if needsAIReview {
			resolved := vit.MergeWithAI(projectDir, branch, baseFiles, preMergeFiles, theirsFiles, issues, []string{})
			if resolved {
				exitOnGitError(vit.GitAdd(projectDir, []string{"timeline/", "assets/"}))
				_, err := vit.GitCommit(projectDir, fmt.Sprintf("vit: AI-resolved merge of '%s'", branch))
				exitOnGitError(err)
				fmt.Println("  Merge complete with AI resolution.")
			} else {
				if hasErrors {
					fmt.Println("  Merge completed with unresolved issues. Review manually.")
				} else {
					fmt.Println("  AI review declined. Merge completed.")
				}
			}
		} else if hasErrors {
			fmt.Println("  Merge completed with issues. Review manually.")
		}
	} else {
		fmt.Println("  Git merge has conflicts.")
		conflicted, err := vit.GitListConflictedFiles(projectDir)
		exitOnGitError(err)
		if len(conflicted) > 0 {
			fmt.Printf("  Conflicted files: %s\n", strings.Join(conflicted, ", "))
		}

		if noAI {
			fmt.Println("  Resolve conflicts manually, then run 'vit commit'.")
			return
		}

		fmt.Println("\n  Attempting AI-assisted conflict resolution...")
		mergeBaseRef, hasBase, _ := vit.GitMergeBase(projectDir, current, branch)
		baseFiles := map[string]any{}
		if hasBase {
			baseFiles = loadFilesAtRef(projectDir, mergeBaseRef)
		}
		theirsFiles := loadFilesAtRef(projectDir, branch)

		exitOnGitError(vit.GitMergeAbort(projectDir))

		resolved := vit.MergeWithAI(projectDir, branch, baseFiles, preMergeFiles, theirsFiles, nil, conflicted)
		if resolved {
			exitOnGitError(vit.GitAdd(projectDir, []string{"timeline/", "assets/"}))
			_, err := vit.GitCommit(projectDir, fmt.Sprintf("vit: AI-resolved merge of '%s'", branch))
			exitOnGitError(err)
			fmt.Println("  Merge complete with AI resolution.")
		} else {
			fmt.Println("  AI merge failed. Resolve conflicts manually.")
			// Re-attempt the merge so user can resolve
			_, _, _ = vit.GitMerge(projectDir, branch)
		}
	}
}

func stringOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func cmdDiff(args []string) {
	_, pos, err := parseArgs(args, map[string]string{}, map[string]bool{})
	exitOnGitError(err)
	projectDir := requireProject()

	ref := "HEAD"
	if len(pos) > 0 {
		ref = pos[0]
	}

	output, err := vit.DiffFromProject(projectDir, ref)
	if err != nil {
		// No commits yet or other git issue — show raw git diff
		raw, rawErr := vit.GitDiff(projectDir, "")
		if rawErr == nil && raw != "" {
			fmt.Print(raw)
		} else {
			fmt.Println("  No changes.")
		}
		return
	}
	if strings.TrimSpace(output) != "" {
		fmt.Println(output)
	} else {
		fmt.Println("  No changes.")
	}
}

func cmdLog(args []string) {
	flags, _, err := parseArgs(args,
		map[string]string{"-n": "count", "--count": "count", "--summary": "summary"},
		map[string]bool{"count": true})
	exitOnGitError(err)
	projectDir := requireProject()

	count := 20
	if c, ok := flags["count"]; ok {
		count, err = strconv.Atoi(c)
		if err != nil {
			fmt.Printf("Error: invalid count: %s\n", c)
			os.Exit(1)
		}
	}

	output, err := vit.GitLog(projectDir, count)
	exitOnGitError(err)
	if output != "" {
		fmt.Print(output)
		if flags["summary"] == "true" {
			summary := vit.SummarizeLog(output)
			if summary != "" {
				fmt.Printf("\n  AI Summary: %s\n", summary)
			} else {
				fmt.Println("\n  AI summary unavailable (check GEMINI_API_KEY).")
			}
		}
	} else {
		fmt.Println("  No commits yet.")
	}
}

func cmdStatus(args []string) {
	projectDir := requireProject()
	current, err := vit.GitCurrentBranch(projectDir)
	exitOnGitError(err)
	fmt.Printf("  Branch: %s\n", current)

	status, err := vit.GitStatus(projectDir)
	exitOnGitError(err)
	if status != "" {
		fmt.Print(status)
	} else {
		fmt.Println("  Working tree clean.")
	}
}

func cmdRevert(args []string) {
	projectDir := requireProject()
	if err := vit.GitRevert(projectDir); err != nil {
		fmt.Printf("  Error: %s\n", err.Error())
		return
	}
	fmt.Println("  Reverted last commit.")
}

func isGitHubAuthError(errStr string) bool {
	markers := []string{
		"invalid username or token",
		"password authentication is not supported",
		"authentication failed",
		"could not read username",
		"could not read password",
		"403",
		"remote: forbidden",
	}
	lower := strings.ToLower(errStr)
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func cmdPush(args []string) {
	flags, _, err := parseArgs(args,
		map[string]string{"--remote": "remote", "--branch": "branch"},
		map[string]bool{"remote": true, "branch": true})
	exitOnGitError(err)
	projectDir := requireProject()

	remote := flags["remote"]
	if remote == "" {
		remote = "origin"
	}
	output, err := vit.GitPush(projectDir, remote, flags["branch"])
	if err != nil {
		errStr := err.Error()
		fmt.Printf("  Error: %s\n", errStr)
		if isGitHubAuthError(errStr) {
			fmt.Println()
			fmt.Println("  GitHub auth failed. SSH is the recommended fix:")
			fmt.Println("    1. ssh-keygen -t ed25519 -C \"your@email.com\"")
			fmt.Println("    2. Add ~/.ssh/id_ed25519.pub at https://github.com/settings/ssh/new")
			fmt.Println("    3. git remote set-url origin git@github.com:user/repo.git")
			fmt.Println("  Or re-run 'vit collab setup' for a guided walkthrough.")
		}
		return
	}
	fmt.Printf("  Pushed to %s\n", remote)
	if strings.TrimSpace(output) != "" {
		fmt.Println(strings.TrimRight(output, "\n"))
	}
}

func cmdPull(args []string) {
	flags, _, err := parseArgs(args,
		map[string]string{"--remote": "remote", "--branch": "branch"},
		map[string]bool{"remote": true, "branch": true})
	exitOnGitError(err)
	projectDir := requireProject()

	remote := flags["remote"]
	if remote == "" {
		remote = "origin"
	}
	output, err := vit.GitPull(projectDir, remote, flags["branch"])
	if err != nil {
		fmt.Printf("  Error: %s\n", err.Error())
		return
	}
	fmt.Printf("  Pulled from %s\n", remote)
	if strings.TrimSpace(output) != "" {
		fmt.Println(strings.TrimRight(output, "\n"))
	}
}

func cmdValidate(args []string) {
	projectDir := requireProject()
	issues, err := vit.ValidateProject(projectDir)
	exitOnGitError(err)
	if len(issues) > 0 {
		fmt.Println(vit.FormatIssues(issues))
		os.Exit(1)
	}
	fmt.Println("  Validation passed — no issues found.")
}

func cmdClone(args []string) {
	_, pos, err := parseArgs(args, map[string]string{}, map[string]bool{})
	exitOnGitError(err)
	if len(pos) == 0 {
		fmt.Println("Error: clone requires a remote URL.")
		os.Exit(1)
	}
	url := pos[0]
	dest := ""
	if len(pos) > 1 {
		dest = pos[1]
	}
	if dest == "" {
		dest = filepath.Base(strings.TrimSuffix(strings.TrimRight(url, "/"), ".git"))
	}
	if _, err := os.Stat(dest); err == nil {
		fmt.Printf("  Error: '%s' already exists.\n", dest)
		os.Exit(1)
	}
	fmt.Printf("  Cloning %s into '%s'...\n", url, dest)
	if err := vit.GitClone(url, dest); err != nil {
		fmt.Printf("  Error: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("  Cloned into '%s'\n", dest)
	fmt.Println("  Note: Media files are not included. Open the project in Resolve and relink any offline clips.")
	fmt.Printf("  Run 'vit checkout main' inside '%s' to restore the latest timeline.\n", dest)
}

func cmdRemote(args []string) {
	projectDir := requireProject()

	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "add":
		if len(args) < 3 {
			fmt.Println("Error: remote add requires a name and URL.")
			os.Exit(1)
		}
		exitOnGitError(vit.GitRemoteAdd(projectDir, args[1], args[2]))
		fmt.Printf("  Added remote '%s' -> %s\n", args[1], args[2])
	case "remove":
		if len(args) < 2 {
			fmt.Println("Error: remote remove requires a name.")
			os.Exit(1)
		}
		exitOnGitError(vit.GitRemoteRemove(projectDir, args[1]))
		fmt.Printf("  Removed remote '%s'\n", args[1])
	case "list", "":
		remotes, err := vit.GitRemoteList(projectDir)
		exitOnGitError(err)
		if len(remotes) > 0 {
			for _, r := range remotes {
				fmt.Printf("  %s\t%s\n", r["name"], r["url"])
			}
		} else {
			fmt.Println("  No remotes configured. Run 'vit collab setup' to add one.")
		}
	default:
		fmt.Printf("Error: unknown remote command '%s'.\n", sub)
		os.Exit(1)
	}
}

var githubHTTPSRe = regexp.MustCompile(`^https://github\.com/([^/]+)/(.+)$`)

func httpsToSSHURL(url string) string {
	if m := githubHTTPSRe.FindStringSubmatch(url); m != nil {
		return fmt.Sprintf("git@github.com:%s/%s", m[1], m[2])
	}
	return url
}

func printSSHInstructions(url, remoteName string) {
	sshURL := httpsToSSHURL(url)
	fmt.Println()
	fmt.Println("  GitHub no longer accepts passwords over HTTPS.")
	fmt.Println("  SSH is the recommended way to authenticate — set it up once and it")
	fmt.Println("  works for every GitHub repo on this machine, with no expiry.")
	fmt.Println()
	fmt.Println("  Step 1 — Generate an SSH key (skip if you already have one):")
	fmt.Println("    ssh-keygen -t ed25519 -C \"your@email.com\"")
	fmt.Println("    (press Enter through all prompts to accept defaults)")
	fmt.Println()
	fmt.Println("  Step 2 — Add your public key to GitHub:")
	fmt.Println("    cat ~/.ssh/id_ed25519.pub")
	fmt.Println("    Copy the output, then go to:")
	fmt.Println("    https://github.com/settings/ssh/new")
	fmt.Println("    Paste it in and save.")
	fmt.Println()
	fmt.Println("  Step 3 — Use the SSH remote URL instead of HTTPS.")
	if sshURL != url {
		fmt.Printf("  Your URL:     %s\n", url)
		fmt.Printf("  SSH version:  %s\n", sshURL)
		fmt.Println()
		fmt.Println("  Update it with:")
		fmt.Printf("    git remote set-url %s %s\n", remoteName, sshURL)
	} else {
		fmt.Println("  Use the SSH URL from GitHub: git@github.com:username/repo.git")
		fmt.Println("  (On the repo page, click Code -> SSH to copy it)")
	}
	fmt.Println()
	fmt.Println("  Then re-run: vit collab setup")
}

func cmdCollabSetup() {
	projectDir := requireProject()

	fmt.Println("  Vit Collaboration Setup")
	fmt.Println("  ─────────────────────────────────────")
	fmt.Println("  Tip: use the SSH URL from GitHub (git@github.com:user/repo.git),")
	fmt.Println("  not the HTTPS URL. SSH works without entering credentials every time.")
	fmt.Println()

	remotes, err := vit.GitRemoteList(projectDir)
	exitOnGitError(err)
	if len(remotes) > 0 {
		fmt.Println("  Existing remotes:")
		for _, r := range remotes {
			fmt.Printf("    %s  %s\n", r["name"], r["url"])
		}
		fmt.Println()
	}

	url := input("  Remote URL (e.g. git@github.com:you/film.git): ")
	if url == "" {
		fmt.Println("  Cancelled.")
		return
	}

	if strings.HasPrefix(url, "https://") {
		sshURL := httpsToSSHURL(url)
		fmt.Println()
		fmt.Println("  Note: you entered an HTTPS URL. SSH is recommended to avoid auth issues.")
		if sshURL != url {
			fmt.Printf("  SSH equivalent: %s\n", sshURL)
			choice := strings.ToLower(input("  Switch to SSH URL? [Y/n]: "))
			if choice != "n" {
				url = sshURL
				fmt.Printf("  Using SSH URL: %s\n", url)
			}
		}
		fmt.Println()
	}

	remoteName := "origin"
	if len(remotes) > 0 {
		remoteName = input("  Remote name [origin]: ")
		if remoteName == "" {
			remoteName = "origin"
		}
	}

	ensureGitIdentity(projectDir)

	exists := false
	for _, r := range remotes {
		if r["name"] == remoteName {
			exists = true
			break
		}
	}
	if !exists {
		exitOnGitError(vit.GitRemoteAdd(projectDir, remoteName, url))
		fmt.Printf("  Added remote '%s'\n", remoteName)
	}

	currentBranch, err := vit.GitCurrentBranch(projectDir)
	exitOnGitError(err)
	fmt.Printf("  Pushing '%s' to %s...\n", currentBranch, remoteName)
	output, err := vit.GitPushSetUpstream(projectDir, remoteName, currentBranch)
	if err != nil {
		errStr := err.Error()
		fmt.Printf("  Push failed: %s\n", errStr)
		if isGitHubAuthError(errStr) {
			printSSHInstructions(url, remoteName)
		} else {
			fmt.Println("  Make sure the remote repository exists and is empty, then try again.")
		}
		return
	}
	if strings.TrimSpace(output) != "" {
		fmt.Println(strings.TrimRight(output, "\n"))
	}

	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Println("  Share this command with collaborators:")
	fmt.Printf("    vit clone %s\n", url)
	fmt.Println()
	fmt.Println("  Each collaborator should:")
	fmt.Println("    1. Run: vit clone <url>")
	fmt.Println("    2. Open the project folder in DaVinci Resolve")
	fmt.Println("    3. Relink any offline media files")
	fmt.Println("    4. Create their own branch: vit branch <name>")
}

// --- Resolve script installation --------------------------------------------

func resolveScriptsDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"),
			"Blackmagic Design", "DaVinci Resolve", "Fusion", "Scripts", "Edit")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home,
			"Library", "Application Support", "Blackmagic Design", "DaVinci Resolve", "Fusion", "Scripts", "Edit")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "DaVinciResolve", "Fusion", "Scripts", "Edit")
	}
}

var resolveScriptNames = []string{"vit_panel.py"}

var resolveMenuNames = map[string]string{"vit_panel.py": "Vit.py"}

// findPackageDir locates the vit repo root (must contain resolve_plugin/ and
// the Python vit/ package that the Resolve scripts import).
func findPackageDir() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".vit", "vit-src"))
	}
	for _, dir := range candidates {
		if isDir(filepath.Join(dir, "resolve_plugin")) && isDir(filepath.Join(dir, "vit")) {
			return dir
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func cmdInstallResolve() {
	packageDir := findPackageDir()
	if packageDir == "" {
		fmt.Println("  Error: resolve_plugin/ directory not found.")
		fmt.Println("  Run this from the vit repo root, or install to ~/.vit/vit-src first.")
		os.Exit(1)
	}
	pluginDir := filepath.Join(packageDir, "resolve_plugin")

	scriptsDir := resolveScriptsDir()
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		fmt.Printf("  Error: %v\n", err)
		os.Exit(1)
	}

	for _, scriptName := range resolveScriptNames {
		source := filepath.Join(pluginDir, scriptName)
		if _, err := os.Stat(source); err != nil {
			fmt.Printf("  Warning: %s not found, skipping.\n", scriptName)
			continue
		}

		menuName := resolveMenuNames[scriptName]
		if menuName == "" {
			menuName = scriptName
		}
		dest := filepath.Join(scriptsDir, menuName)

		_ = os.Remove(dest)

		var err error
		if runtime.GOOS == "windows" {
			err = copyFile(source, dest)
		} else {
			err = os.Symlink(source, dest)
		}
		if err != nil {
			fmt.Printf("  Error linking %s: %v\n", menuName, err)
			os.Exit(1)
		}
		fmt.Printf("  Linked: %s -> %s\n", menuName, source)
	}

	// Save the repo root path so Resolve scripts can find the vit package
	home, _ := os.UserHomeDir()
	vitUserDir := filepath.Join(home, ".vit")
	if err := os.MkdirAll(vitUserDir, 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(vitUserDir, "package_path"), []byte(packageDir), 0o644)
		fmt.Printf("  Saved package path: %s\n", packageDir)
	}

	fmt.Printf("\n  Installed %d script(s) to Resolve.\n", len(resolveScriptNames))
	fmt.Println("  Restart Resolve, then run Workspace > Scripts > Vit for the unified panel.")
}

func cmdUninstallResolve() {
	// Include legacy script names so old installs get cleaned up
	allVitNames := []string{
		"Vit.py",
		"Vit - Panel.py",
		"Vit - Save Version.py",
		"Vit - New Branch.py",
		"Vit - Merge Branch.py",
		"Vit - Switch Branch.py",
		"Vit - Status.py",
		"Vit - Push.py",
		"Vit - Pull & Restore.py",
	}
	scriptsDir := resolveScriptsDir()
	removed := 0
	for _, menuName := range allVitNames {
		dest := filepath.Join(scriptsDir, menuName)
		if _, err := os.Lstat(dest); err == nil {
			if os.Remove(dest) == nil {
				fmt.Printf("  Removed: %s\n", menuName)
				removed++
			}
		}
	}
	if removed > 0 {
		fmt.Printf("\n  Uninstalled %d scripts from Resolve.\n", removed)
	} else {
		fmt.Println("  No vit scripts found in Resolve.")
	}
}

func printHelp() {
	fmt.Println("usage: vit <command> [args]")
	fmt.Println()
	fmt.Println("Git for Video Editing — version control timeline metadata")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init [path]              Initialize a new vit project")
	fmt.Println("  add                      Stage timeline files")
	fmt.Println("  commit [-m MSG]          Save a version")
	fmt.Println("  branch [name] [-l]       Create or list branches")
	fmt.Println("  checkout <ref>           Switch branch or version")
	fmt.Println("  merge <branch> [--no-ai] Merge a branch")
	fmt.Println("  diff [ref]               Show timeline changes")
	fmt.Println("  log [-n N] [--summary]   Show version history")
	fmt.Println("  status                   Show project status")
	fmt.Println("  revert                   Revert last version")
	fmt.Println("  push [--remote R] [--branch B]  Push to remote")
	fmt.Println("  pull [--remote R] [--branch B]  Pull from remote")
	fmt.Println("  validate                 Validate timeline consistency")
	fmt.Println("  clone <url> [dir]        Clone a remote vit project")
	fmt.Println("  remote [add|list|remove] Manage remote repositories")
	fmt.Println("  collab setup             Guided remote setup wizard")
	fmt.Println("  install-resolve          Install scripts into DaVinci Resolve")
	fmt.Println("  uninstall-resolve        Remove scripts from DaVinci Resolve")
	fmt.Println()
	fmt.Println("  --version                Show version")
}

func jsonNumber(s string) any {
	return json.Number(s)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printHelp()
		os.Exit(0)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "--version", "-V":
		fmt.Printf("vit %s\n", version)
	case "-h", "--help", "help":
		printHelp()
	case "init":
		cmdInit(rest)
	case "add":
		cmdAdd(rest)
	case "commit":
		cmdCommit(rest)
	case "branch":
		cmdBranch(rest)
	case "checkout":
		cmdCheckout(rest)
	case "merge":
		cmdMerge(rest)
	case "diff":
		cmdDiff(rest)
	case "log":
		cmdLog(rest)
	case "status":
		cmdStatus(rest)
	case "revert":
		cmdRevert(rest)
	case "push":
		cmdPush(rest)
	case "pull":
		cmdPull(rest)
	case "validate":
		cmdValidate(rest)
	case "clone":
		cmdClone(rest)
	case "remote":
		cmdRemote(rest)
	case "collab":
		if len(rest) > 0 && rest[0] == "setup" {
			cmdCollabSetup()
		} else {
			fmt.Println("usage: vit collab setup")
		}
	case "install-resolve":
		cmdInstallResolve()
	case "uninstall-resolve":
		cmdUninstallResolve()
	case "internal":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "usage: vit internal <op>")
			os.Exit(2)
		}
		os.Exit(vit.RunInternal(rest[0]))
	default:
		fmt.Printf("vit: unknown command '%s'\n", cmd)
		printHelp()
		os.Exit(2)
	}
}
