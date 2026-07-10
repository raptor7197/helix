package vit

import (
	"os"
	"path/filepath"
	"testing"
)

// Files written by the Go binary must be byte-identical to Python's
// json.dump(indent=2, sort_keys=True) + "\n" so git diffs stay clean when
// both sides touch the same file.
func TestWriteJSONFilePythonParity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	// Round-trip: read a Python-style file, write it back — must be identical.
	pythonWritten := "{\n  \"frame_rate\": 24.0,\n  \"name\": \"caf\\u00e9 <cut> & more\",\n  \"track_count\": {\n    \"audio\": 1,\n    \"video\": 2\n  }\n}\n"
	if err := os.WriteFile(path, []byte(pythonWritten), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ReadJSONFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONFile(path, m); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != pythonWritten {
		t.Errorf("round-trip not byte-identical:\nwant: %q\ngot:  %q", pythonWritten, string(got))
	}
}

func TestReadJSONFileMissingReturnsEmpty(t *testing.T) {
	m, err := ReadJSONFile(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestEncodePyEscapesAstralPlane(t *testing.T) {
	s, err := EncodePy(map[string]any{"emoji": "🎬"})
	if err != nil {
		t.Fatal(err)
	}
	// Python: json.dumps({"emoji": "🎬"}) → surrogate pair 🎬
	if s != "{\n  \"emoji\": \"\\ud83c\\udfac\"\n}" {
		t.Errorf("astral escaping wrong: %q", s)
	}
}

func TestPyFloat(t *testing.T) {
	cases := map[float64]string{
		50:     "50.0",
		50.5:   "50.5",
		0.5:    "0.5",
		100:    "100.0",
		-2:     "-2.0",
		1.25e8: "1.25e+08",
	}
	for f, want := range cases {
		if got := pyFloat(f); got != want {
			t.Errorf("pyFloat(%v) = %s, want %s", f, got, want)
		}
	}
}
