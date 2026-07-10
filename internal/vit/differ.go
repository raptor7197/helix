// Human-readable diff formatting for timeline changes.
// Direct port of vit/differ.py (plus the name maps from vit/models.py).
package vit

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var CompositeModeNames = map[int]string{
	0: "normal", 1: "add", 2: "subtract", 3: "difference",
	4: "multiply", 5: "screen", 6: "overlay", 7: "hard_light",
	8: "soft_light", 9: "darken", 10: "lighten", 11: "color_dodge",
	12: "color_burn", 13: "exclusion", 14: "hue", 15: "saturate",
	16: "colorize", 17: "luma_mask", 18: "divide", 19: "linear_dodge",
	20: "linear_burn", 21: "linear_light", 22: "vivid_light",
	23: "pin_light", 24: "hard_mix", 25: "lighter_color",
	26: "darker_color", 27: "foreground", 28: "alpha",
	29: "inverted_alpha", 30: "lum", 31: "inverted_lum",
}

var DynamicZoomEaseNames = map[int]string{
	0: "linear", 1: "ease_in", 2: "ease_out", 3: "ease_in_and_out",
}

var RetimeProcessNames = map[int]string{
	0: "project_default", 1: "nearest", 2: "frame_blend", 3: "optical_flow",
}

var MotionEstNames = map[int]string{
	0: "project_default", 1: "standard_faster", 2: "standard_better",
	3: "enhanced_faster", 4: "enhanced_better", 5: "speed_warp",
}

// FramesToTimecode converts a frame number to HH:MM:SS:FF.
func FramesToTimecode(frames float64, fps float64) string {
	totalFrames := int(frames)
	ifps := int(fps)
	ff := totalFrames % ifps
	totalSeconds := totalFrames / ifps
	ss := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	mm := totalMinutes % 60
	hh := totalMinutes / 60
	return fmt.Sprintf("%02d:%02d:%02d:%02d", hh, mm, ss, ff)
}

func framesToDuration(frames float64, fps float64) string {
	seconds := frames / fps
	if seconds < 1 {
		return fmt.Sprintf("%df", int(frames))
	}
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(seconds / 60)
	secs := math.Mod(seconds, 60)
	return fmt.Sprintf("%dm%.0fs", minutes, secs)
}

// orderedItems flattens track items into first-seen id order with an id map
// (parity with Python dict construction + .items() iteration).
func orderedItems(container map[string]any, tracksKey string) ([]string, map[string]map[string]any) {
	order := []string{}
	byID := map[string]map[string]any{}
	for _, track := range getSlice(container, tracksKey) {
		tm, _ := track.(map[string]any)
		for _, item := range getSlice(tm, "items") {
			im, _ := item.(map[string]any)
			id := pyStr(im["id"])
			if _, seen := byID[id]; !seen {
				order = append(order, id)
			}
			byID[id] = im
		}
	}
	return order, byID
}

var transformKeys = []string{
	"Pan", "Tilt", "ZoomX", "ZoomY", "Opacity",
	"RotationAngle", "AnchorPointX", "AnchorPointY",
	"Pitch", "Yaw", "FlipX", "FlipY",
	"CropLeft", "CropRight", "CropTop", "CropBottom",
	"CropSoftness", "CropRetain", "Distortion",
}

func transformDefault(key string) any {
	if key == "FlipX" || key == "FlipY" || key == "CropRetain" {
		return false
	}
	return json.Number("0")
}

func getOrDefault(m map[string]any, key string, def any) any {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// DiffCuts diffs cuts.json and returns human-readable lines.
func DiffCuts(old, new map[string]any, fps float64) []string {
	lines := []string{}
	oldOrder, oldItems := orderedItems(old, "video_tracks")
	newOrder, newItems := orderedItems(new, "video_tracks")

	for _, itemID := range newOrder {
		if _, ok := oldItems[itemID]; !ok {
			item := newItems[itemID]
			tc := FramesToTimecode(numFloat(item["record_start_frame"], 0), fps)
			dur := framesToDuration(
				numFloat(item["record_end_frame"], 0)-numFloat(item["record_start_frame"], 0), fps)
			lines = append(lines, fmt.Sprintf(
				"  + Added clip '%s' on V%s at %s (%s)",
				pyStr(item["name"]), pyStr(item["track_index"]), tc, dur))
		}
	}

	for _, itemID := range oldOrder {
		if _, ok := newItems[itemID]; !ok {
			item := oldItems[itemID]
			lines = append(lines, fmt.Sprintf(
				"  - Removed clip '%s' from V%s",
				pyStr(item["name"]), pyStr(item["track_index"])))
		}
	}

	for _, itemID := range newOrder {
		oldItem, ok := oldItems[itemID]
		if !ok {
			continue
		}
		newItem := newItems[itemID]
		name := pyStr(newItem["name"])

		if !jsonEqual(oldItem["record_start_frame"], newItem["record_start_frame"]) {
			lines = append(lines, fmt.Sprintf("  ~ Trimmed '%s' start: %s → %s", name,
				FramesToTimecode(numFloat(oldItem["record_start_frame"], 0), fps),
				FramesToTimecode(numFloat(newItem["record_start_frame"], 0), fps)))
		}
		if !jsonEqual(oldItem["record_end_frame"], newItem["record_end_frame"]) {
			lines = append(lines, fmt.Sprintf("  ~ Trimmed '%s' end: %s → %s", name,
				FramesToTimecode(numFloat(oldItem["record_end_frame"], 0), fps),
				FramesToTimecode(numFloat(newItem["record_end_frame"], 0), fps)))
		}
		if !jsonEqual(oldItem["track_index"], newItem["track_index"]) {
			lines = append(lines, fmt.Sprintf("  ~ Moved '%s' from V%s to V%s", name,
				pyStr(oldItem["track_index"]), pyStr(newItem["track_index"])))
		}

		oldT := getMap(oldItem, "transform")
		newT := getMap(newItem, "transform")
		for _, key := range transformKeys {
			def := transformDefault(key)
			oldV := getOrDefault(oldT, key, def)
			newV := getOrDefault(newT, key, def)
			if !jsonEqual(oldV, newV) {
				lines = append(lines, fmt.Sprintf("  ~ clip '%s': %s %s → %s", name, key, pyStr(oldV), pyStr(newV)))
			}
		}

		oldCM := numInt(oldItem["composite_mode"], 0)
		newCM := numInt(newItem["composite_mode"], 0)
		if oldCM != newCM {
			lines = append(lines, fmt.Sprintf("  ~ clip '%s': Composite %s → %s", name,
				nameOr(CompositeModeNames, oldCM, "mode"), nameOr(CompositeModeNames, newCM, "mode")))
		}

		oldDZ := numInt(oldItem["dynamic_zoom_ease"], 0)
		newDZ := numInt(newItem["dynamic_zoom_ease"], 0)
		if oldDZ != newDZ {
			lines = append(lines, fmt.Sprintf("  ~ clip '%s': Dynamic Zoom %s → %s", name,
				nameOr(DynamicZoomEaseNames, oldDZ, "ease"), nameOr(DynamicZoomEaseNames, newDZ, "ease")))
		}

		oldEn := boolOrDefault(oldItem["clip_enabled"], true)
		newEn := boolOrDefault(newItem["clip_enabled"], true)
		if oldEn != newEn {
			state := "disabled"
			if newEn {
				state = "enabled"
			}
			lines = append(lines, fmt.Sprintf("  ~ clip '%s': %s", name, state))
		}

		lines = append(lines, diffSpeed(oldItem, newItem, name)...)
	}
	return lines
}

func nameOr(names map[int]string, key int, prefix string) string {
	if n, ok := names[key]; ok {
		return n
	}
	return fmt.Sprintf("%s(%d)", prefix, key)
}

func boolOrDefault(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if v == nil {
		return def
	}
	if f, ok := numFloatOk(v); ok {
		return f != 0
	}
	return def
}

func diffSpeed(oldItem, newItem map[string]any, clipName string) []string {
	lines := []string{}
	oldSpeed := getMap(oldItem, "speed")
	newSpeed := getMap(newItem, "speed")

	oldPct := numFloat(oldSpeed["speed_percent"], 100.0)
	newPct := numFloat(newSpeed["speed_percent"], 100.0)
	if oldPct != newPct {
		lines = append(lines, fmt.Sprintf("  ~ clip '%s': Speed %s → %s", clipName,
			formatSpeed(oldSpeed["speed_percent"], oldPct),
			formatSpeed(newSpeed["speed_percent"], newPct)))
	}

	oldRT := numInt(oldSpeed["retime_process"], 0)
	newRT := numInt(newSpeed["retime_process"], 0)
	if oldRT != newRT {
		lines = append(lines, fmt.Sprintf("  ~ clip '%s': Retime method %s → %s", clipName,
			nameOr(RetimeProcessNames, oldRT, "unknown"), nameOr(RetimeProcessNames, newRT, "unknown")))
	}

	oldME := numInt(oldSpeed["motion_estimation"], 0)
	newME := numInt(newSpeed["motion_estimation"], 0)
	if oldME != newME {
		lines = append(lines, fmt.Sprintf("  ~ clip '%s': Motion estimation %s → %s", clipName,
			nameOr(MotionEstNames, oldME, "unknown"), nameOr(MotionEstNames, newME, "unknown")))
	}
	return lines
}

// formatSpeed formats a speed percentage like differ._format_speed. raw is
// the JSON value (nil when the default 100.0 applied), pct its numeric value.
func formatSpeed(raw any, pct float64) string {
	if pct == 100.0 {
		return "100% (normal)"
	}
	literal := pyFloat(pct)
	if raw != nil {
		literal = pyStr(raw)
	}
	multiplier := strconv.FormatFloat(pct/100.0, 'g', 2, 64)
	if pct > 100 {
		return fmt.Sprintf("%s%% (%sx fast)", literal, multiplier)
	}
	return fmt.Sprintf("%s%% (%sx slow)", literal, multiplier)
}

func formatRGB(vals []any) string {
	if len(vals) != 3 {
		return fmt.Sprintf("%v", vals)
	}
	return fmt.Sprintf("R:%.3f G:%.3f B:%.3f",
		numFloat(vals[0], 0), numFloat(vals[1], 0), numFloat(vals[2], 0))
}

func formatWheel(wheel map[string]any) string {
	if len(wheel) == 0 {
		return fmt.Sprintf("%v", wheel)
	}
	parts := []string{}
	labels := map[string]string{"r": "R", "g": "G", "b": "B", "y": "Y"}
	for _, ch := range []string{"r", "g", "b", "y"} {
		if v, ok := wheel[ch]; ok {
			parts = append(parts, fmt.Sprintf("%s:%.3f", labels[ch], numFloat(v, 0)))
		}
	}
	return strings.Join(parts, " ")
}

func diffWheelChannels(oldNode, newNode map[string]any, prefix, wheel, label string) []string {
	channels := [][2]string{{"r", "R"}, {"g", "G"}, {"b", "B"}, {"m", "M"}}
	changed := []string{}
	for _, ch := range channels {
		key := wheel + "_" + ch[0]
		oldV, newV := oldNode[key], newNode[key]
		if !jsonEqual(oldV, newV) && (oldV != nil || newV != nil) {
			oldS, newS := "default", "default"
			if oldV != nil {
				oldS = fmt.Sprintf("%+.4f", numFloat(oldV, 0))
			}
			if newV != nil {
				newS = fmt.Sprintf("%+.4f", numFloat(newV, 0))
			}
			changed = append(changed, fmt.Sprintf("%s: %s → %s", ch[1], oldS, newS))
		}
	}
	if len(changed) > 0 {
		return []string{fmt.Sprintf("%s: %s  %s", prefix, label, strings.Join(changed, "  "))}
	}
	return nil
}

func diffNodeValues(oldNode, newNode map[string]any, itemID string, nodeIdx int) []string {
	lines := []string{}
	prefix := fmt.Sprintf("  ~ clip '%s' node %d", itemID, nodeIdx)

	for _, kv := range [][2]string{{"slope", "Slope"}, {"offset", "Offset"}, {"power", "Power"}} {
		oldV, newV := oldNode[kv[0]], newNode[kv[0]]
		if !jsonEqual(oldV, newV) {
			oldS, newS := "default", "default"
			if s, ok := oldV.([]any); ok && len(s) > 0 {
				oldS = formatRGB(s)
			}
			if s, ok := newV.([]any); ok && len(s) > 0 {
				newS = formatRGB(s)
			}
			lines = append(lines, fmt.Sprintf("%s: %s %s → %s", prefix, kv[1], oldS, newS))
		}
	}

	for _, kv := range [][2]string{{"saturation", "Saturation"}, {"contrast", "Contrast"},
		{"pivot", "Pivot"}, {"hue", "Hue"}, {"color_boost", "Color Boost"}} {
		oldV, newV := oldNode[kv[0]], newNode[kv[0]]
		if !jsonEqual(oldV, newV) && (oldV != nil || newV != nil) {
			oldS, newS := "default", "default"
			if oldV != nil {
				oldS = fmt.Sprintf("%.3f", numFloat(oldV, 0))
			}
			if newV != nil {
				newS = fmt.Sprintf("%.3f", numFloat(newV, 0))
			}
			lines = append(lines, fmt.Sprintf("%s: %s %s → %s", prefix, kv[1], oldS, newS))
		}
	}

	for _, kv := range [][2]string{{"lift", "Lift"}, {"gamma", "Gamma"},
		{"gain", "Gain"}, {"color_offset", "Offset"}} {
		oldV, newV := oldNode[kv[0]], newNode[kv[0]]
		if !jsonEqual(oldV, newV) && (oldV != nil || newV != nil) {
			oldS, newS := "default", "default"
			if m, ok := oldV.(map[string]any); ok && len(m) > 0 {
				oldS = formatWheel(m)
			}
			if m, ok := newV.(map[string]any); ok && len(m) > 0 {
				newS = formatWheel(m)
			}
			lines = append(lines, fmt.Sprintf("%s: %s %s → %s", prefix, kv[1], oldS, newS))
		}
	}

	for _, kv := range [][2]string{{"lift", "Lift"}, {"gamma", "Gamma"}, {"gain", "Gain"}, {"offset", "Offset"}} {
		lines = append(lines, diffWheelChannels(oldNode, newNode, prefix, kv[0], kv[1])...)
	}

	for _, kv := range [][2]string{{"temperature", "Temperature"}, {"tint", "Tint"}} {
		oldV, newV := oldNode[kv[0]], newNode[kv[0]]
		if !jsonEqual(oldV, newV) && (oldV != nil || newV != nil) {
			suffix := ""
			if kv[0] == "temperature" {
				suffix = "K"
			}
			oldS, newS := "default", "default"
			if oldV != nil {
				oldS = fmt.Sprintf("%.0f", numFloat(oldV, 0))
			}
			if newV != nil {
				newS = fmt.Sprintf("%.0f", numFloat(newV, 0))
			}
			lines = append(lines, fmt.Sprintf("%s: %s %s%s → %s%s", prefix, kv[1], oldS, suffix, newS, suffix))
		}
	}

	for _, kv := range [][2]string{{"sharpness", "Sharpness"},
		{"noise_reduction_luma", "NR Luma"}, {"noise_reduction_chroma", "NR Chroma"}} {
		oldV, newV := oldNode[kv[0]], newNode[kv[0]]
		if !jsonEqual(oldV, newV) && (oldV != nil || newV != nil) {
			oldS, newS := "default", "default"
			if oldV != nil {
				oldS = fmt.Sprintf("%.3f", numFloat(oldV, 0))
			}
			if newV != nil {
				newS = fmt.Sprintf("%.3f", numFloat(newV, 0))
			}
			lines = append(lines, fmt.Sprintf("%s: %s %s → %s", prefix, kv[1], oldS, newS))
		}
	}

	oldLut := getString(oldNode, "lut")
	newLut := getString(newNode, "lut")
	if oldLut != newLut {
		o, n := oldLut, newLut
		if o == "" {
			o = "none"
		}
		if n == "" {
			n = "none"
		}
		lines = append(lines, fmt.Sprintf("%s: LUT '%s' → '%s'", prefix, o, n))
	}
	return lines
}

// DiffColor diffs color.json and returns human-readable lines.
func DiffColor(old, new map[string]any) []string {
	lines := []string{}
	oldGrades := getMap(old, "grades")
	newGrades := getMap(new, "grades")

	for _, itemID := range sortedKeys(newGrades) {
		newG, _ := newGrades[itemID].(map[string]any)
		if _, ok := oldGrades[itemID]; !ok {
			lines = append(lines, fmt.Sprintf("  + Added color grade for clip '%s'", itemID))
			for _, node := range getSlice(newG, "nodes") {
				nm, _ := node.(map[string]any)
				for _, kv := range [][2]string{{"slope", "Slope"}, {"saturation", "Saturation"},
					{"contrast", "Contrast"}, {"hue", "Hue"}} {
					val := nm[kv[0]]
					if val == nil {
						continue
					}
					if s, ok := val.([]any); ok {
						lines = append(lines, fmt.Sprintf("    %s: %s", kv[1], formatRGB(s)))
					} else {
						lines = append(lines, fmt.Sprintf("    %s: %.3f", kv[1], numFloat(val, 0)))
					}
				}
			}
			continue
		}

		oldG, _ := oldGrades[itemID].(map[string]any)
		for _, key := range []string{"num_nodes", "version_name", "drx_file"} {
			if !jsonEqual(oldG[key], newG[key]) {
				lines = append(lines, fmt.Sprintf("  ~ clip '%s': %s %s → %s",
					itemID, key, pyStr(oldG[key]), pyStr(newG[key])))
			}
		}

		oldNodes := getSlice(oldG, "nodes")
		newNodes := getSlice(newG, "nodes")
		maxNodes := len(oldNodes)
		if len(newNodes) > maxNodes {
			maxNodes = len(newNodes)
		}
		for idx := 0; idx < maxNodes; idx++ {
			if idx >= len(oldNodes) {
				lines = append(lines, fmt.Sprintf("  + clip '%s': added node %d", itemID, idx+1))
				continue
			}
			if idx >= len(newNodes) {
				lines = append(lines, fmt.Sprintf("  - clip '%s': removed node %d", itemID, idx+1))
				continue
			}
			on, _ := oldNodes[idx].(map[string]any)
			nn, _ := newNodes[idx].(map[string]any)
			lines = append(lines, diffNodeValues(on, nn, itemID, idx+1)...)
		}
	}

	for _, itemID := range sortedKeys(oldGrades) {
		if _, ok := newGrades[itemID]; !ok {
			lines = append(lines, fmt.Sprintf("  - Removed color grade for clip '%s'", itemID))
		}
	}
	return lines
}

// DiffAudio diffs audio.json and returns human-readable lines.
func DiffAudio(old, new map[string]any, fps float64) []string {
	lines := []string{}
	oldOrder, oldItems := orderedItems(old, "audio_tracks")
	newOrder, newItems := orderedItems(new, "audio_tracks")

	for _, itemID := range newOrder {
		item := newItems[itemID]
		oldItem, ok := oldItems[itemID]
		if !ok {
			lines = append(lines, fmt.Sprintf("  + Added audio clip '%s'", itemID))
			continue
		}
		for _, key := range []string{"volume", "pan"} {
			if !jsonEqual(oldItem[key], item[key]) {
				lines = append(lines, fmt.Sprintf("  ~ audio '%s': %s %s → %s",
					itemID, key, pyStr(oldItem[key]), pyStr(item[key])))
			}
		}
		lines = append(lines, diffSpeed(oldItem, item, itemID)...)
	}

	for _, itemID := range oldOrder {
		if _, ok := newItems[itemID]; !ok {
			lines = append(lines, fmt.Sprintf("  - Removed audio clip '%s'", itemID))
		}
	}
	return lines
}

// DiffMarkers diffs markers.json (markers keyed by frame, numerically).
func DiffMarkers(old, new map[string]any, fps float64) []string {
	lines := []string{}
	collect := func(m map[string]any) ([]float64, map[float64]map[string]any) {
		order := []float64{}
		byFrame := map[float64]map[string]any{}
		for _, marker := range getSlice(m, "markers") {
			mm, _ := marker.(map[string]any)
			frame := numFloat(mm["frame"], 0)
			if _, seen := byFrame[frame]; !seen {
				order = append(order, frame)
			}
			byFrame[frame] = mm
		}
		return order, byFrame
	}
	oldOrder, oldMarkers := collect(old)
	newOrder, newMarkers := collect(new)

	for _, frame := range newOrder {
		marker := newMarkers[frame]
		if oldM, ok := oldMarkers[frame]; !ok {
			lines = append(lines, fmt.Sprintf("  + Added marker at %s: \"%s\"",
				FramesToTimecode(frame, fps), getString(marker, "name")))
		} else if !jsonEqual(any(oldM), any(marker)) {
			lines = append(lines, fmt.Sprintf("  ~ Modified marker at %s: \"%s\"",
				FramesToTimecode(frame, fps), getString(marker, "name")))
		}
	}
	for _, frame := range oldOrder {
		if _, ok := newMarkers[frame]; !ok {
			lines = append(lines, fmt.Sprintf("  - Removed marker at %s", FramesToTimecode(frame, fps)))
		}
	}
	return lines
}

// DiffMetadata diffs metadata.json.
func DiffMetadata(old, new map[string]any) []string {
	lines := []string{}
	for _, key := range []string{"project_name", "timeline_name", "frame_rate", "start_timecode"} {
		if !jsonEqual(old[key], new[key]) {
			lines = append(lines, fmt.Sprintf("  ~ %s: %s → %s", key, pyStr(old[key]), pyStr(new[key])))
		}
	}
	oldRes := getMap(old, "resolution")
	newRes := getMap(new, "resolution")
	if !jsonEqual(any(oldRes), any(newRes)) {
		lines = append(lines, fmt.Sprintf("  ~ resolution: %sx%s → %sx%s",
			pyStr(oldRes["width"]), pyStr(oldRes["height"]),
			pyStr(newRes["width"]), pyStr(newRes["height"])))
	}
	return lines
}

// FormatDiff formats a complete human-readable diff across all domain files.
func FormatDiff(oldFiles, newFiles map[string]any, timelineName, branchInfo string) string {
	fps := numFloat(getMap(newFiles, "metadata")["frame_rate"], 24.0)

	outputLines := []string{}
	if timelineName != "" {
		outputLines = append(outputLines, "  Timeline: "+timelineName)
	}
	if branchInfo != "" {
		outputLines = append(outputLines, "  Branch: "+branchInfo)
	}
	if len(outputLines) > 0 {
		outputLines = append(outputLines, "")
	}

	sections := []struct {
		name string
		key  string
		fn   func(old, new map[string]any) []string
	}{
		{"CUTS", "cuts", func(o, n map[string]any) []string { return DiffCuts(o, n, fps) }},
		{"COLOR", "color", DiffColor},
		{"AUDIO", "audio", func(o, n map[string]any) []string { return DiffAudio(o, n, fps) }},
		{"MARKERS", "markers", func(o, n map[string]any) []string { return DiffMarkers(o, n, fps) }},
		{"METADATA", "metadata", DiffMetadata},
	}

	hasChanges := false
	for _, section := range sections {
		oldData := getMap(oldFiles, section.key)
		newData := getMap(newFiles, section.key)
		if jsonEqual(any(oldData), any(newData)) {
			continue
		}
		diffLines := section.fn(oldData, newData)
		if len(diffLines) > 0 {
			hasChanges = true
			outputLines = append(outputLines, "  "+section.name+":")
			outputLines = append(outputLines, diffLines...)
			outputLines = append(outputLines, "")
		}
	}
	if !hasChanges {
		outputLines = append(outputLines, "  No changes.")
	}
	return strings.Join(outputLines, "\n")
}

func loadFilesAtRef(projectDir, ref string, domains map[string]string, order []string) (map[string]any, error) {
	files := map[string]any{}
	for _, domain := range order {
		content, ok, err := GitShowFile(projectDir, ref, domains[domain])
		if err != nil {
			return nil, err
		}
		if ok {
			v, err := DecodeJSON([]byte(content))
			if err != nil {
				files[domain] = map[string]any{}
				continue
			}
			if m, isMap := v.(map[string]any); isMap {
				files[domain] = m
			} else {
				files[domain] = map[string]any{}
			}
		} else {
			files[domain] = map[string]any{}
		}
	}
	return files, nil
}

var diffDomains = map[string]string{
	"cuts":     "timeline/cuts.json",
	"color":    "timeline/color.json",
	"audio":    "timeline/audio.json",
	"markers":  "timeline/markers.json",
	"metadata": "timeline/metadata.json",
}

var diffDomainOrder = []string{"cuts", "color", "audio", "markers", "metadata"}

// DiffFromProject generates a human-readable diff between current state and a
// git ref.
func DiffFromProject(projectDir, ref string) (string, error) {
	oldFiles, err := loadFilesAtRef(projectDir, ref, diffDomains, diffDomainOrder)
	if err != nil {
		return "", err
	}
	newFiles := map[string]any{}
	for _, domain := range diffDomainOrder {
		m, err := ReadJSONFile(filepath.Join(projectDir, diffDomains[domain]))
		if err != nil {
			return "", err
		}
		newFiles[domain] = m
	}
	timelineName := getString(getMap(newFiles, "metadata"), "timeline_name")
	return FormatDiff(oldFiles, newFiles, timelineName, ""), nil
}

var categoryDomains = map[string]string{
	"cuts":  "timeline/cuts.json",
	"color": "timeline/color.json",
	"audio": "timeline/audio.json",
}

var categoryDomainOrder = []string{"cuts", "color", "audio"}

// GetChangesByCategory returns changes categorized by domain (audio, video,
// color); each entry has id, name, type, details.
func GetChangesByCategory(projectDir, ref string) (map[string][]map[string]any, error) {
	oldFiles, err := loadFilesAtRef(projectDir, ref, categoryDomains, categoryDomainOrder)
	if err != nil {
		return nil, err
	}
	newFiles := map[string]any{}
	for _, domain := range categoryDomainOrder {
		m, err := ReadJSONFile(filepath.Join(projectDir, categoryDomains[domain]))
		if err != nil {
			return nil, err
		}
		newFiles[domain] = m
	}

	changes := map[string][]map[string]any{"audio": {}, "video": {}, "color": {}}

	oldVOrder, oldVideo := orderedItems(getMap(oldFiles, "cuts"), "video_tracks")
	newVOrder, newVideo := orderedItems(getMap(newFiles, "cuts"), "video_tracks")
	for _, itemID := range newVOrder {
		item := newVideo[itemID]
		name := itemID
		if n := getString(item, "name"); n != "" {
			name = n
		}
		if oldItem, ok := oldVideo[itemID]; !ok {
			trackIdx := "?"
			if v, present := item["track_index"]; present {
				trackIdx = pyStr(v)
			}
			changes["video"] = append(changes["video"], map[string]any{
				"id": itemID, "name": name, "type": "added",
				"details": "Added to V" + trackIdx,
			})
		} else if !jsonEqual(any(item), any(oldItem)) {
			changes["video"] = append(changes["video"], map[string]any{
				"id": itemID, "name": name, "type": "modified",
				"details": "Trimmed or moved",
			})
		}
	}
	for _, itemID := range oldVOrder {
		if _, ok := newVideo[itemID]; !ok {
			item := oldVideo[itemID]
			name := itemID
			if n := getString(item, "name"); n != "" {
				name = n
			}
			changes["video"] = append(changes["video"], map[string]any{
				"id": itemID, "name": name, "type": "removed", "details": "Removed",
			})
		}
	}

	oldAOrder, oldAudio := orderedItems(getMap(oldFiles, "audio"), "audio_tracks")
	newAOrder, newAudio := orderedItems(getMap(newFiles, "audio"), "audio_tracks")
	for _, itemID := range newAOrder {
		if oldItem, ok := oldAudio[itemID]; !ok {
			changes["audio"] = append(changes["audio"], map[string]any{
				"id": itemID, "name": itemID, "type": "added", "details": "Added audio clip",
			})
		} else if !jsonEqual(any(newAudio[itemID]), any(oldItem)) {
			changes["audio"] = append(changes["audio"], map[string]any{
				"id": itemID, "name": itemID, "type": "modified", "details": "Volume or pan changed",
			})
		}
	}
	for _, itemID := range oldAOrder {
		if _, ok := newAudio[itemID]; !ok {
			changes["audio"] = append(changes["audio"], map[string]any{
				"id": itemID, "name": itemID, "type": "removed", "details": "Removed",
			})
		}
	}

	oldGrades := getMap(getMap(oldFiles, "color"), "grades")
	newGrades := getMap(getMap(newFiles, "color"), "grades")
	for _, itemID := range sortedKeys(newGrades) {
		if oldGrade, ok := oldGrades[itemID]; !ok {
			changes["color"] = append(changes["color"], map[string]any{
				"id": itemID, "name": itemID, "type": "added", "details": "Added color grade",
			})
		} else if !jsonEqual(newGrades[itemID], oldGrade) {
			changes["color"] = append(changes["color"], map[string]any{
				"id": itemID, "name": itemID, "type": "modified", "details": "Grade modified",
			})
		}
	}
	for _, itemID := range sortedKeys(oldGrades) {
		if _, ok := newGrades[itemID]; !ok {
			changes["color"] = append(changes["color"], map[string]any{
				"id": itemID, "name": itemID, "type": "removed", "details": "Grade removed",
			})
		}
	}

	return changes, nil
}

func countDomainChanges(old, new map[string]any, domain string) []map[string]any {
	changes := []map[string]any{}
	appendChange := func(id, name, typ string) {
		changes = append(changes, map[string]any{"id": id, "name": name, "type": typ})
	}

	switch domain {
	case "cuts", "audio":
		tracksKey := "video_tracks"
		useName := true
		if domain == "audio" {
			tracksKey = "audio_tracks"
			useName = false
		}
		oldOrder, oldItems := orderedItems(old, tracksKey)
		newOrder, newItems := orderedItems(new, tracksKey)
		nameOf := func(id string, item map[string]any) string {
			if useName {
				if n := getString(item, "name"); n != "" {
					return n
				}
			}
			return id
		}
		for _, id := range newOrder {
			if oldItem, ok := oldItems[id]; !ok {
				appendChange(id, nameOf(id, newItems[id]), "added")
			} else if !jsonEqual(any(newItems[id]), any(oldItem)) {
				appendChange(id, nameOf(id, newItems[id]), "modified")
			}
		}
		for _, id := range oldOrder {
			if _, ok := newItems[id]; !ok {
				appendChange(id, nameOf(id, oldItems[id]), "removed")
			}
		}
	case "color":
		oldGrades := getMap(old, "grades")
		newGrades := getMap(new, "grades")
		for _, id := range sortedKeys(newGrades) {
			if oldGrade, ok := oldGrades[id]; !ok {
				appendChange(id, id, "added")
			} else if !jsonEqual(newGrades[id], oldGrade) {
				appendChange(id, id, "modified")
			}
		}
		for _, id := range sortedKeys(oldGrades) {
			if _, ok := newGrades[id]; !ok {
				appendChange(id, id, "removed")
			}
		}
	}
	return changes
}

// GetBranchDiffByCategory compares two branches against their merge base and
// returns categorized changes for each.
func GetBranchDiffByCategory(projectDir, branchA, branchB string) (map[string][]map[string]any, map[string][]map[string]any, error) {
	base, hasBase, err := GitMergeBase(projectDir, branchA, branchB)
	if err != nil {
		return nil, nil, err
	}

	baseFiles := map[string]any{}
	if hasBase {
		baseFiles, err = loadFilesAtRef(projectDir, base, categoryDomains, categoryDomainOrder)
		if err != nil {
			return nil, nil, err
		}
	}
	aFiles, err := loadFilesAtRef(projectDir, branchA, categoryDomains, categoryDomainOrder)
	if err != nil {
		return nil, nil, err
	}
	bFiles, err := loadFilesAtRef(projectDir, branchB, categoryDomains, categoryDomainOrder)
	if err != nil {
		return nil, nil, err
	}

	categorize := func(files map[string]any) map[string][]map[string]any {
		return map[string][]map[string]any{
			"video": countDomainChanges(getMap(baseFiles, "cuts"), getMap(files, "cuts"), "cuts"),
			"audio": countDomainChanges(getMap(baseFiles, "audio"), getMap(files, "audio"), "audio"),
			"color": countDomainChanges(getMap(baseFiles, "color"), getMap(files, "color"), "color"),
		}
	}
	return categorize(aFiles), categorize(bFiles), nil
}

// DetectOverlappingDomains lists domains modified by both branches relative
// to base (port of cli._detect_overlapping_domains).
func DetectOverlappingDomains(baseFiles, oursFiles, theirsFiles map[string]any) []string {
	overlapping := []string{}
	keys := make([]string, 0, len(baseFiles))
	for _, domain := range DomainOrder {
		if _, ok := baseFiles[domain]; ok {
			keys = append(keys, domain)
		}
	}
	// include any non-standard keys deterministically
	extra := []string{}
	for k := range baseFiles {
		found := false
		for _, d := range keys {
			if d == k {
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)

	orEmpty := func(v any) any {
		if v == nil {
			return map[string]any{}
		}
		return v
	}
	for _, domain := range keys {
		baseContent := orEmpty(baseFiles[domain])
		oursChanged := !jsonEqual(baseContent, orEmpty(oursFiles[domain]))
		theirsChanged := !jsonEqual(baseContent, orEmpty(theirsFiles[domain]))
		if oursChanged && theirsChanged {
			overlapping = append(overlapping, domain)
		}
	}
	return overlapping
}
