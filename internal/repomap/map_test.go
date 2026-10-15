package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oc-ops/oc-ops/internal/contract"
)

func TestMapTree(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "pkg", "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte("package pkg\ntype Foo struct{}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "pkg", "sub", "b.go"), []byte("package sub\nfunc Bar(){}\n"), 0o644)

	resp, err := Handler(contract.Request{Tool: "map", Args: map[string]any{"mode": "tree", "depth": 6}, Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("status: %+v", resp)
	}
	var kinds []string
	for _, item := range resp.Items {
		var m map[string]any
		_ = jsonUnmarshal(item, &m)
		if m["kind"] == "file" {
			kinds = append(kinds, m["path"].(string))
		}
	}
	if len(kinds) == 0 {
		t.Fatal("expected file entries in tree")
	}
}

func TestMapSymbols(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello(){}\ntype World struct{}\n"), 0o644)
	resp, err := Handler(contract.Request{Tool: "map", Args: map[string]any{"mode": "symbols"}, Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range resp.Items {
		var m map[string]any
		_ = jsonUnmarshal(item, &m)
		names[m["name"].(string)] = true
	}
	if !names["Hello"] || !names["World"] {
		t.Fatalf("expected symbols Hello and World, got %v", names)
	}
}

func TestMapSkipsGit(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc A(){}\n"), 0o644)
	resp, err := Handler(contract.Request{Tool: "map", Args: map[string]any{"mode": "tree", "depth": 6}, Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range resp.Items {
		var m map[string]any
		_ = jsonUnmarshal(item, &m)
		if contains(m["path"].(string), ".git") {
			t.Fatal("map should skip .git")
		}
	}
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
