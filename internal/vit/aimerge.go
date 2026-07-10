// AI-assisted semantic merge and enrichment (Gemini).
//
// NOTE: This is a redacted stub. The AI merge analyzer, prompt construction,
// interactive resolution flow, and Gemini REST client are proprietary and
// withheld from this distribution. The public interface is preserved so the
// project builds and runs; without the engine, these entry points degrade to
// the same safe fallbacks the real code uses when no API key is present.
// The full implementation is available under NDA.
package vit

import "fmt"

// MergeWithAI runs the full interactive AI merge flow.
//
// Redacted stub: reports that the engine is unavailable and declines,
// leaving the merge for manual resolution.
func MergeWithAI(projectDir, branch string, baseFiles, oursFiles, theirsFiles map[string]any, issues []ValidationIssue, conflictedFiles []string) bool {
	fmt.Println("  AI merge engine is not included in this distribution.")
	return false
}

// AnalyzeBranchComparison analyzes two branches and recommends a merge
// strategy.
//
// Redacted stub: returns the change-count heuristic recommendation.
func AnalyzeBranchComparison(branchA, branchB string, changesA, changesB map[string][]map[string]any) map[string]any {
	count := func(c map[string][]map[string]any) int {
		n := 0
		for _, items := range c {
			n += len(items)
		}
		return n
	}
	aCount, bCount := count(changesA), count(changesB)
	rec, explanation := "manual_review", "AI analysis engine not included in this distribution; recommend manual review."
	if aCount == 0 && bCount > 0 {
		rec, explanation = "accept_b", fmt.Sprintf("Only %s has changes.", branchB)
	} else if bCount == 0 && aCount > 0 {
		rec, explanation = "accept_a", fmt.Sprintf("Only %s has changes.", branchA)
	}
	return map[string]any{
		"summary_a":      fmt.Sprintf("%d changes", aCount),
		"summary_b":      fmt.Sprintf("%d changes", bCount),
		"conflicts":      []any{},
		"recommendation": rec,
		"explanation":    explanation,
	}
}

// ClassifyCommitType classifies a commit as audio, video, or color.
//
// Redacted stub: uses the deterministic file-path heuristic (CategorizeCommit).
func ClassifyCommitType(commitHash string, filesChanged []string, message string) string {
	return CategorizeCommit(filesChanged)
}

// SuggestCommitMessage suggests a commit message from a timeline diff.
//
// Redacted stub: no suggestion without the engine.
func SuggestCommitMessage(diffText string) string {
	return ""
}

// SummarizeLog summarizes recent commit history.
//
// Redacted stub: no summary without the engine.
func SummarizeLog(commitsText string) string {
	return ""
}
