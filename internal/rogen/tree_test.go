package rogen

import (
	"encoding/json"
	"testing"
)

func TestGetOrCreateNode(t *testing.T) {
	parent := map[string]any{}
	created := getOrCreateNode(parent, "MyFolder", "Folder")
	if created["$className"] != "Folder" {
		t.Errorf("$className = %v", created["$className"])
	}

	existing := map[string]any{"$path": "src/MyFolder"}
	parent = map[string]any{"MyFolder": existing}
	got := getOrCreateNode(parent, "MyFolder", "Folder")
	if got["$path"] != "src/MyFolder" {
		t.Errorf("existing node replaced: %v", got)
	}
	if _, ok := got["$className"]; ok {
		t.Error("existing node gained a $className")
	}
}

func TestMarshalSortedJSON(t *testing.T) {
	value := map[string]any{
		"Zebra": map[string]any{"B": json.Number("1"), "A": json.Number("2")},
		"Apple": map[string]any{"D": true, "C": "x&y"},
		"List":  []any{"b", "a"},
		"Empty": map[string]any{},
	}
	got, err := marshalSortedJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "Apple": {
    "C": "x&y",
    "D": true
  },
  "Empty": {},
  "List": [
    "b",
    "a"
  ],
  "Zebra": {
    "A": 2,
    "B": 1
  }
}
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPathHasPrefix(t *testing.T) {
	cases := []struct {
		p, dir string
		want   bool
	}{
		{"build/foo.luau", "build", true},
		{"build", "build", true},
		{"build2/foo.luau", "build", false},
		{"include", "build", false},
		{"rojo/generated/build/x", "rojo/generated/build", true},
	}
	for _, c := range cases {
		if got := pathHasPrefix(c.p, c.dir); got != c.want {
			t.Errorf("pathHasPrefix(%q, %q) = %v, want %v", c.p, c.dir, got, c.want)
		}
	}
}
