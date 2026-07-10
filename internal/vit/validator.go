// Post-merge validation.
//
// NOTE: This is a redacted stub. The validation engine (orphaned-reference
// detection, clip-overlap checks, audio/video sync and speed consistency) is
// proprietary and withheld from this distribution. The public interface is
// preserved so the project builds, runs, and can be evaluated end to end.
// The full implementation is available under NDA.
package vit

// ValidationIssue is a single post-merge validation finding.
type ValidationIssue struct {
	Severity string         `json:"severity"` // "error" or "warning"
	Category string         `json:"category"` // "orphaned_ref", "sync", "overlap", ...
	Message  string         `json:"message"`
	Details  map[string]any `json:"details"`
}

// ValidateProject runs all post-merge validation checks on the project state.
//
// Redacted stub: returns no issues. The real implementation is withheld.
func ValidateProject(projectDir string) ([]ValidationIssue, error) {
	return []ValidationIssue{}, nil
}

// FormatIssues renders validation issues for display.
//
// Redacted stub.
func FormatIssues(issues []ValidationIssue) string {
	if len(issues) == 0 {
		return "  No issues found."
	}
	return "  (validation output withheld from this distribution)"
}
