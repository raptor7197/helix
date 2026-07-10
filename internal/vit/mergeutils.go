// Deterministic overlay-aware merge policy for timeline domain files.
//
// NOTE: This is a redacted stub. The merge-conflict resolution engine (title
// and media id-collision handling, V2 overlay promotion, sidecar rename
// bookkeeping, and track-count reconciliation) is the core proprietary logic
// of vit and is withheld from this distribution. The public interface is
// preserved so the project builds and runs; the full implementation is
// available under NDA.
package vit

// OverlayMergePlan is the bookkeeping for title-overlay remaps created during
// a merge.
type OverlayMergePlan struct {
	IDRemaps         map[string]string `json:"id_remaps"`
	GeneratorRenames map[string]string `json:"generator_renames"`
	GradeRenames     map[string]string `json:"grade_renames"`
	GradeRestoreOurs []string          `json:"grade_restore_ours"`
}

// MergeTimelineDomainsForOverlays normalizes title/media id collisions so
// titles become overlays.
//
// Redacted stub: returns the merged input unchanged with an empty plan.
// The real conflict-resolution implementation is withheld.
func MergeTimelineDomainsForOverlays(mergedFiles, oursFiles, theirsFiles map[string]any) (map[string]any, *OverlayMergePlan) {
	plan := &OverlayMergePlan{
		IDRemaps:         map[string]string{},
		GeneratorRenames: map[string]string{},
		GradeRenames:     map[string]string{},
		GradeRestoreOurs: []string{},
	}
	return mergedFiles, plan
}

// ReferencedSidecars returns the referenced generator and grade sidecar paths.
//
// Redacted stub: returns empty sets.
func ReferencedSidecars(domainFiles map[string]any) (generators, grades map[string]bool) {
	return map[string]bool{}, map[string]bool{}
}
