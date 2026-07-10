// Package vit is the Go port of the vit core: git wrapper, domain-file IO,
// validation, merge helpers, human-readable diffs, and AI merge.
//
// JSON encoding mirrors Python's json.dump(indent=2, sort_keys=True,
// ensure_ascii=True) so files written by the Go binary and by the Python
// serializer produce identical git diffs.
package vit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DecodeJSON parses JSON keeping numbers as json.Number so their literal
// representation (e.g. "24.0" vs "24") survives a read-modify-write cycle.
func DecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// ReadJSONFile reads a JSON object file, returning an empty map if the file
// does not exist (parity with json_writer.read_json).
func ReadJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	v, err := DecodeJSON(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: not a JSON object", path)
	}
	return m, nil
}

// EncodePy renders v like Python's json.dumps(v, indent=2, sort_keys=True):
// sorted keys, two-space indent, no HTML escaping, non-ASCII escaped as \uXXXX,
// no trailing newline.
func EncodePy(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	s := strings.TrimSuffix(buf.String(), "\n")
	return escapeNonASCII(s), nil
}

// escapeNonASCII rewrites runes above 0x7F as \uXXXX escapes (surrogate pairs
// for astral-plane runes), matching Python's default ensure_ascii=True.
func escapeNonASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r > 0xFFFF:
			r -= 0x10000
			hi := 0xD800 + (r >> 10)
			lo := 0xDC00 + (r & 0x3FF)
			fmt.Fprintf(&b, "\\u%04x\\u%04x", hi, lo)
		default:
			fmt.Fprintf(&b, "\\u%04x", r)
		}
	}
	return b.String()
}

// WriteJSONFile writes v with Python-parity formatting plus a trailing
// newline, creating parent directories (parity with json_writer._write_json).
func WriteJSONFile(path string, v any) error {
	s, err := EncodePy(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(s+"\n"), 0o644)
}

// DomainFileMap mirrors merge_utils.domain_file_map.
func DomainFileMap() map[string]string {
	return map[string]string{
		"cuts":     "timeline/cuts.json",
		"color":    "timeline/color.json",
		"audio":    "timeline/audio.json",
		"effects":  "timeline/effects.json",
		"markers":  "timeline/markers.json",
		"metadata": "timeline/metadata.json",
		"manifest": "assets/manifest.json",
	}
}

// DomainOrder is the canonical domain iteration order (Python dict insertion
// order of domain_file_map / read_all_domain_files).
var DomainOrder = []string{"cuts", "color", "audio", "effects", "markers", "metadata", "manifest"}

// ReadAllDomainFiles mirrors json_writer.read_all_domain_files.
func ReadAllDomainFiles(projectDir string) (map[string]any, error) {
	out := map[string]any{}
	for _, domain := range DomainOrder {
		m, err := ReadJSONFile(filepath.Join(projectDir, DomainFileMap()[domain]))
		if err != nil {
			return nil, err
		}
		out[domain] = m
	}
	return out, nil
}

// --- generic JSON value helpers -------------------------------------------

func getMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func getSlice(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// numFloatOk converts a JSON scalar to float64 when it is numeric.
func numFloatOk(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case bool: // Python: True == 1, False == 0
		if n {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func numFloat(v any, def float64) float64 {
	if f, ok := numFloatOk(v); ok {
		return f
	}
	return def
}

func numInt(v any, def int) int {
	if f, ok := numFloatOk(v); ok {
		return int(f)
	}
	return def
}

// pyStr renders a JSON value like Python's str(): None, True/False, numeric
// literals verbatim, strings raw.
func pyStr(v any) string {
	switch n := v.(type) {
	case nil:
		return "None"
	case bool:
		if n {
			return "True"
		}
		return "False"
	case json.Number:
		return n.String()
	case string:
		return n
	case int:
		return strconv.Itoa(n)
	case float64:
		return pyFloat(n)
	}
	return fmt.Sprintf("%v", v)
}

// pyFloat renders a float like Python's str(float): integral values keep one
// decimal ("50.0"), others use the shortest representation.
func pyFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// jsonEqual compares two decoded JSON values with Python == semantics:
// numbers compare numerically regardless of int/float representation, and
// bools equal 0/1.
func jsonEqual(a, b any) bool {
	if fa, ok := numFloatOk(a); ok {
		if fb, ok := numFloatOk(b); ok {
			// Python bool vs bool is also numeric-equal, so this covers it.
			return fa == fb
		}
		return false
	}
	switch va := a.(type) {
	case nil:
		return b == nil
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case []any:
		vb, ok := b.([]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !jsonEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		vb, ok := b.(map[string]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for k, v := range va {
			bv, present := vb[k]
			if !present || !jsonEqual(v, bv) {
				return false
			}
		}
		return true
	}
	return false
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopy(val)
		}
		return out
	default:
		return v
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
