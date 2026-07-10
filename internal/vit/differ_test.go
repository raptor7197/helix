package vit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFramesToTimecode(t *testing.T) {
	cases := []struct {
		frames float64
		want   string
	}{
		{0, "00:00:00:00"},
		{24, "00:00:01:00"},
		{240, "00:00:10:00"},
		{1440, "00:01:00:00"},
		{12, "00:00:00:12"},
	}
	for _, c := range cases {
		if got := FramesToTimecode(c.frames, 24.0); got != c.want {
			t.Errorf("FramesToTimecode(%v) = %s, want %s", c.frames, got, c.want)
		}
	}
}

func track(items ...any) map[string]any {
	return map[string]any{"video_tracks": []any{map[string]any{"index": 1, "items": items}}}
}

func audioTrack(items ...any) map[string]any {
	return map[string]any{"audio_tracks": []any{map[string]any{"index": 1, "items": items}}}
}

func TestDiffCutsAddedClip(t *testing.T) {
	old := track()
	new := track(map[string]any{
		"id": "item_001", "name": "Interview.mov", "track_index": 1,
		"record_start_frame": 240, "record_end_frame": 480,
	})
	lines := DiffCuts(old, new, 24.0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "+ Added clip") || !strings.Contains(lines[0], "Interview.mov") {
		t.Errorf("unexpected line: %s", lines[0])
	}
	if !strings.Contains(lines[0], "at 00:00:10:00 (10.0s)") {
		t.Errorf("timecode/duration mismatch: %s", lines[0])
	}
}

func TestDiffCutsRemovedClip(t *testing.T) {
	old := track(map[string]any{
		"id": "item_001", "name": "OldClip.mov", "track_index": 1,
		"record_start_frame": 0, "record_end_frame": 100,
	})
	new := track()
	lines := DiffCuts(old, new, 24.0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "- Removed clip") || !strings.Contains(lines[0], "OldClip.mov") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestDiffCutsTrimmedClip(t *testing.T) {
	old := track(map[string]any{
		"id": "item_001", "name": "Clip.mov", "track_index": 1,
		"record_start_frame": 0, "record_end_frame": 720,
	})
	new := track(map[string]any{
		"id": "item_001", "name": "Clip.mov", "track_index": 1,
		"record_start_frame": 0, "record_end_frame": 684,
	})
	lines := DiffCuts(old, new, 24.0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "Trimmed") || !strings.Contains(lines[0], "end") {
		t.Errorf("unexpected line: %s", lines[0])
	}
	if !strings.Contains(lines[0], "00:00:30:00 → 00:00:28:12") {
		t.Errorf("timecodes wrong: %s", lines[0])
	}
}

func TestDiffColorChanged(t *testing.T) {
	old := map[string]any{"grades": map[string]any{"item_001": map[string]any{
		"num_nodes":    1,
		"nodes":        []any{map[string]any{"index": 1, "label": "", "lut": ""}},
		"version_name": "", "drx_file": nil,
	}}}
	new := map[string]any{"grades": map[string]any{"item_001": map[string]any{
		"num_nodes": 2,
		"nodes": []any{
			map[string]any{"index": 1, "label": "", "lut": ""},
			map[string]any{"index": 2, "label": "LUT", "lut": "Rec709.cube"},
		},
		"version_name": "", "drx_file": nil,
	}}}
	lines := DiffColor(old, new)
	if len(lines) < 1 {
		t.Fatal("expected at least 1 line")
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "num_nodes") || strings.Contains(strings.ToLower(l), "node") || strings.Contains(l, "LUT") {
			found = true
		}
	}
	if !found {
		t.Errorf("no change detected: %v", lines)
	}
}

func TestDiffMarkersAdded(t *testing.T) {
	old := map[string]any{"markers": []any{}}
	new := map[string]any{"markers": []any{
		map[string]any{"frame": 240, "color": "Blue", "name": "Fix here", "note": "", "duration": 1},
	}}
	lines := DiffMarkers(old, new, 24.0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "+ Added marker") || !strings.Contains(lines[0], "Fix here") {
		t.Errorf("unexpected line: %s", lines[0])
	}
	if !strings.Contains(lines[0], "00:00:10:00") {
		t.Errorf("timecode wrong: %s", lines[0])
	}
}

func TestDiffMarkersRemoved(t *testing.T) {
	old := map[string]any{"markers": []any{
		map[string]any{"frame": 240, "color": "Blue", "name": "Old", "note": "", "duration": 1},
	}}
	new := map[string]any{"markers": []any{}}
	lines := DiffMarkers(old, new, 24.0)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "- Removed marker") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestDiffMetadataChanged(t *testing.T) {
	old := map[string]any{"project_name": "Old Name", "frame_rate": json.Number("24.0")}
	new := map[string]any{"project_name": "New Name", "frame_rate": json.Number("24.0")}
	lines := DiffMetadata(old, new)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], "project_name") {
		t.Errorf("unexpected line: %s", lines[0])
	}
	if !strings.Contains(lines[0], "Old Name → New Name") {
		t.Errorf("values wrong: %s", lines[0])
	}
}

func TestDiffCutsSpeedChanged(t *testing.T) {
	old := track(map[string]any{
		"id": "item_001", "name": "Action.mov", "track_index": 1,
		"record_start_frame": 0, "record_end_frame": 480,
	})
	new := track(map[string]any{
		"id": "item_001", "name": "Action.mov", "track_index": 1,
		"record_start_frame": 0, "record_end_frame": 480,
		"speed": map[string]any{
			"speed_percent":  json.Number("50.0"),
			"retime_process": 3,
		},
	})
	lines := DiffCuts(old, new, 24.0)

	speedLines := []string{}
	retimeLines := []string{}
	for _, l := range lines {
		if strings.Contains(l, "Speed") {
			speedLines = append(speedLines, l)
		}
		if strings.Contains(l, "Retime") {
			retimeLines = append(retimeLines, l)
		}
	}
	if len(speedLines) < 1 {
		t.Fatalf("no speed lines: %v", lines)
	}
	if !strings.Contains(speedLines[0], "50.0%") || !strings.Contains(speedLines[0], "slow") {
		t.Errorf("speed line wrong: %s", speedLines[0])
	}
	if !strings.Contains(speedLines[0], "100% (normal) → 50.0% (0.5x slow)") {
		t.Errorf("speed formatting parity broken: %s", speedLines[0])
	}
	if len(retimeLines) != 1 || !strings.Contains(retimeLines[0], "optical_flow") {
		t.Errorf("retime lines wrong: %v", retimeLines)
	}
}

func TestDiffAudioSpeedChanged(t *testing.T) {
	old := audioTrack(map[string]any{"id": "audio_001", "volume": 0.0, "pan": 0.0})
	new := audioTrack(map[string]any{
		"id": "audio_001", "volume": 0.0, "pan": 0.0,
		"speed": map[string]any{"speed_percent": json.Number("200.0")},
	})
	lines := DiffAudio(old, new, 24.0)
	speedLines := []string{}
	for _, l := range lines {
		if strings.Contains(l, "Speed") {
			speedLines = append(speedLines, l)
		}
	}
	if len(speedLines) != 1 {
		t.Fatalf("expected 1 speed line, got %v", lines)
	}
	if !strings.Contains(speedLines[0], "200.0%") || !strings.Contains(speedLines[0], "fast") {
		t.Errorf("speed line wrong: %s", speedLines[0])
	}
	if !strings.Contains(speedLines[0], "(2x fast)") {
		t.Errorf("multiplier formatting parity broken: %s", speedLines[0])
	}
}

func TestFormatDiffNoChanges(t *testing.T) {
	files := map[string]any{
		"cuts": map[string]any{}, "color": map[string]any{}, "audio": map[string]any{},
		"markers": map[string]any{}, "metadata": map[string]any{},
	}
	output := FormatDiff(files, files, "Test", "")
	if !strings.Contains(output, "No changes") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestFormatDiffFull(t *testing.T) {
	oldFiles := map[string]any{
		"cuts":     track(),
		"color":    map[string]any{"grades": map[string]any{}},
		"audio":    map[string]any{"audio_tracks": []any{}},
		"markers":  map[string]any{"markers": []any{}},
		"metadata": map[string]any{"frame_rate": json.Number("24.0")},
	}
	newFiles := map[string]any{
		"cuts": track(map[string]any{
			"id": "item_001", "name": "NewClip.mov", "track_index": 1,
			"record_start_frame": 0, "record_end_frame": 480,
		}),
		"color":    map[string]any{"grades": map[string]any{}},
		"audio":    map[string]any{"audio_tracks": []any{}},
		"markers":  map[string]any{"markers": []any{}},
		"metadata": map[string]any{"frame_rate": json.Number("24.0")},
	}
	output := FormatDiff(oldFiles, newFiles, "Main Edit", "")
	for _, want := range []string{"Timeline: Main Edit", "CUTS", "NewClip.mov"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q: %s", want, output)
		}
	}
}

func TestDetectOverlappingDomains(t *testing.T) {
	base := map[string]any{"cuts": map[string]any{"a": 1}, "color": map[string]any{"b": 2}}
	ours := map[string]any{"cuts": map[string]any{"a": 1}, "color": map[string]any{"b": 3}}
	theirs := map[string]any{"cuts": map[string]any{"a": 2}, "color": map[string]any{"b": 2}}
	if got := DetectOverlappingDomains(base, ours, theirs); len(got) != 0 {
		t.Errorf("expected no overlap, got %v", got)
	}

	ours2 := map[string]any{"cuts": map[string]any{"a": 2}, "color": map[string]any{"b": 2}}
	theirs2 := map[string]any{"cuts": map[string]any{"a": 3}, "color": map[string]any{"b": 2}}
	if got := DetectOverlappingDomains(base, ours2, theirs2); len(got) != 1 || got[0] != "cuts" {
		t.Errorf("expected [cuts], got %v", got)
	}

	base3 := map[string]any{"cuts": map[string]any{"a": 1}, "color": map[string]any{"b": 2}, "audio": map[string]any{"c": 3}}
	ours3 := map[string]any{"cuts": map[string]any{"a": 2}, "color": map[string]any{"b": 3}, "audio": map[string]any{"c": 3}}
	theirs3 := map[string]any{"cuts": map[string]any{"a": 3}, "color": map[string]any{"b": 4}, "audio": map[string]any{"c": 3}}
	got := DetectOverlappingDomains(base3, ours3, theirs3)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "cuts") || !strings.Contains(joined, "color") || strings.Contains(joined, "audio") {
		t.Errorf("expected cuts+color, got %v", got)
	}

	same := map[string]any{"cuts": map[string]any{"a": 1}}
	if got := DetectOverlappingDomains(same, same, same); len(got) != 0 {
		t.Errorf("expected no overlap, got %v", got)
	}
}

// int-vs-float equality must match Python == (1 == 1.0)
func TestJsonEqualNumericParity(t *testing.T) {
	a := map[string]any{"v": json.Number("1")}
	b := map[string]any{"v": json.Number("1.0")}
	if !jsonEqual(any(a), any(b)) {
		t.Error("1 and 1.0 must compare equal")
	}
	if !jsonEqual(json.Number("0"), false) {
		t.Error("0 and False must compare equal (Python semantics)")
	}
}
