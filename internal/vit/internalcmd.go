// The `vit internal` JSON dispatcher. The Python side (vit/core.py and
// friends, imported by the Resolve plugin) shells out to these ops so git,
// diff, validation, merge and AI logic live only in Go.
//
// Protocol: `vit internal <op>` with a JSON payload on stdin; response on
// stdout is {"ok": true, "result": ...} or {"ok": false, "error": "..."}.
package vit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

type internalHandler func(payload map[string]any) (any, error)

func payloadString(p map[string]any, key string) string { return getString(p, key) }

func payloadStrings(p map[string]any, key string) []string {
	out := []string{}
	for _, v := range getSlice(p, key) {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func payloadMap(p map[string]any, key string) map[string]any { return getMap(p, key) }

var internalHandlers = map[string]internalHandler{
	"add": func(p map[string]any) (any, error) {
		return nil, GitAdd(payloadString(p, "project_dir"), payloadStrings(p, "paths"))
	},
	"commit": func(p map[string]any) (any, error) {
		return GitCommit(payloadString(p, "project_dir"), payloadString(p, "message"))
	},
	"branch": func(p map[string]any) (any, error) {
		return nil, GitBranch(payloadString(p, "project_dir"), payloadString(p, "name"))
	},
	"checkout": func(p map[string]any) (any, error) {
		return nil, GitCheckout(payloadString(p, "project_dir"), payloadString(p, "ref"))
	},
	"current-branch": func(p map[string]any) (any, error) {
		return GitCurrentBranch(payloadString(p, "project_dir"))
	},
	"list-branches": func(p map[string]any) (any, error) {
		return GitListBranches(payloadString(p, "project_dir"))
	},
	"merge": func(p map[string]any) (any, error) {
		success, output, err := GitMerge(payloadString(p, "project_dir"), payloadString(p, "branch"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"success": success, "output": output}, nil
	},
	"merge-abort": func(p map[string]any) (any, error) {
		return nil, GitMergeAbort(payloadString(p, "project_dir"))
	},
	"merge-base": func(p map[string]any) (any, error) {
		base, ok, err := GitMergeBase(payloadString(p, "project_dir"), payloadString(p, "ref1"), payloadString(p, "ref2"))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return base, nil
	},
	"is-clean": func(p map[string]any) (any, error) {
		return GitIsClean(payloadString(p, "project_dir"))
	},
	"list-conflicted": func(p map[string]any) (any, error) {
		return GitListConflictedFiles(payloadString(p, "project_dir"))
	},
	"checkout-theirs": func(p map[string]any) (any, error) {
		return nil, GitCheckoutTheirs(payloadString(p, "project_dir"), payloadStrings(p, "paths"))
	},
	"show-file": func(p map[string]any) (any, error) {
		content, ok, err := GitShowFile(payloadString(p, "project_dir"), payloadString(p, "ref"), payloadString(p, "path"))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return content, nil
	},
	"push": func(p map[string]any) (any, error) {
		return GitPush(payloadString(p, "project_dir"), payloadString(p, "remote"), payloadString(p, "branch"))
	},
	"pull": func(p map[string]any) (any, error) {
		return GitPull(payloadString(p, "project_dir"), payloadString(p, "remote"), payloadString(p, "branch"))
	},
	"status": func(p map[string]any) (any, error) {
		return GitStatus(payloadString(p, "project_dir"))
	},
	"log": func(p map[string]any) (any, error) {
		return GitLog(payloadString(p, "project_dir"), numInt(p["max_count"], 20))
	},
	"log-with-changes": func(p map[string]any) (any, error) {
		return GitLogWithChanges(payloadString(p, "project_dir"), numInt(p["max_count"], 20))
	},
	"log-with-topology": func(p map[string]any) (any, error) {
		return GitLogWithTopology(payloadString(p, "project_dir"), numInt(p["max_count"], 30))
	},
	"validate": func(p map[string]any) (any, error) {
		issues, err := ValidateProject(payloadString(p, "project_dir"))
		if err != nil {
			return nil, err
		}
		return issues, nil
	},
	"changes-by-category": func(p map[string]any) (any, error) {
		ref := payloadString(p, "ref")
		if ref == "" {
			ref = "HEAD"
		}
		return GetChangesByCategory(payloadString(p, "project_dir"), ref)
	},
	"branch-diff-by-category": func(p map[string]any) (any, error) {
		changesA, changesB, err := GetBranchDiffByCategory(
			payloadString(p, "project_dir"), payloadString(p, "branch_a"), payloadString(p, "branch_b"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"changes_a": changesA, "changes_b": changesB}, nil
	},
	"overlay-merge": func(p map[string]any) (any, error) {
		merged, plan := MergeTimelineDomainsForOverlays(
			payloadMap(p, "merged"), payloadMap(p, "ours"), payloadMap(p, "theirs"))
		return map[string]any{"merged": merged, "plan": plan}, nil
	},
	"referenced-sidecars": func(p map[string]any) (any, error) {
		generators, grades := ReferencedSidecars(payloadMap(p, "domain_files"))
		return map[string]any{
			"generators": sortedBoolKeys(generators),
			"grades":     sortedBoolKeys(grades),
		}, nil
	},
	"analyze-branch-comparison": func(p map[string]any) (any, error) {
		return AnalyzeBranchComparison(
			payloadString(p, "branch_a"), payloadString(p, "branch_b"),
			changesFromPayload(payloadMap(p, "changes_a")),
			changesFromPayload(payloadMap(p, "changes_b"))), nil
	},
	"classify-commit": func(p map[string]any) (any, error) {
		return ClassifyCommitType(
			payloadString(p, "hash"), payloadStrings(p, "files"), payloadString(p, "message")), nil
	},
	"suggest-commit-message": func(p map[string]any) (any, error) {
		msg := SuggestCommitMessage(payloadString(p, "diff_text"))
		if msg == "" {
			return nil, nil
		}
		return msg, nil
	},
	"summarize-log": func(p map[string]any) (any, error) {
		summary := SummarizeLog(payloadString(p, "commits_text"))
		if summary == "" {
			return nil, nil
		}
		return summary, nil
	},
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func changesFromPayload(m map[string]any) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for key, v := range m {
		items := []map[string]any{}
		if slice, ok := v.([]any); ok {
			for _, item := range slice {
				if im, isMap := item.(map[string]any); isMap {
					items = append(items, im)
				}
			}
		}
		out[key] = items
	}
	return out
}

// RunInternal executes an internal op, reading the JSON payload from stdin
// and writing the JSON response to stdout. Returns the process exit code.
func RunInternal(op string) int {
	handler, ok := internalHandlers[op]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown internal op: %s\n", op)
		return 2
	}

	payload := map[string]any{}
	data, err := io.ReadAll(os.Stdin)
	if err == nil && len(data) > 0 {
		if v, decodeErr := DecodeJSON(data); decodeErr == nil {
			if m, isMap := v.(map[string]any); isMap {
				payload = m
			}
		}
	}

	result, err := handler(payload)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err != nil {
		_ = enc.Encode(map[string]any{"ok": false, "error": err.Error()})
		return 0
	}
	_ = enc.Encode(map[string]any{"ok": true, "result": result})
	return 0
}
