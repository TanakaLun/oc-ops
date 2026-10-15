package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oc-ops/oc-ops/internal/contract"
)

func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "hello world\nfoo bar\n")
	write("b.go", "package main\nfunc main(){}\nfoo()\n")
	write("skip.txt", "node_modules\n")
	write("node_modules/x.txt", "hidden\n")
	write("sub/c.md", "# title\nfoo\n")
	return dir
}

func req(tool string, args map[string]any, cwd string, readOnly bool) contract.Request {
	return contract.Request{Tool: tool, Args: args, Cwd: cwd, ReadOnly: readOnly}
}

func TestGrepSummary(t *testing.T) {
	dir := setup(t)
	resp, err := grep(req("grep", map[string]any{"pattern": "foo", "path": ".", "outputMode": "summary"}, dir, false))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Matched < 3 {
		t.Fatalf("expected matched >= 3, got %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("summary should have 1 item, got %d", len(resp.Items))
	}
}

func TestGrepContentBounded(t *testing.T) {
	dir := setup(t)
	resp, err := grep(req("grep", map[string]any{"pattern": "foo", "path": ".", "outputMode": "content", "limit": 2}, dir, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) > 2 {
		t.Fatalf("content should be bounded to 2, got %d", len(resp.Items))
	}
	if resp.Total < 2 {
		t.Fatalf("expected total >= 2, got %d", resp.Total)
	}
}

func TestGlobIgnoresNodeModules(t *testing.T) {
	dir := setup(t)
	resp, err := glob(req("glob", map[string]any{"path": ".", "pattern": []any{"**"}, "outputMode": "paths"}, dir, false))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range resp.Items {
		var s string
		_ = json.Unmarshal(item, &s)
		if s == "" {
			t.Fatal("glob paths output should be strings")
		}
		if contains(s, "node_modules") {
			t.Fatalf("glob should skip node_modules, got %s", s)
		}
	}
}

func TestInspectRejectsDir(t *testing.T) {
	dir := setup(t)
	resp, _ := inspect(req("inspect", map[string]any{"path": "."}, dir, false))
	if resp.Status != "error" {
		t.Fatalf("inspect on a dir should error, got %s", resp.Status)
	}
}

func TestReplaceDryRunNoWrite(t *testing.T) {
	dir := setup(t)
	before, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	resp, err := replace(req("replace", map[string]any{"path": ".", "pattern": "foo", "replacement": "bar"}, dir, false))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Phase != "observe" {
		t.Fatalf("dry-run should be observe, got %+v", resp)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(before) != string(after) {
		t.Fatal("dry-run must not modify files")
	}
}

func TestReplaceApplyWrites(t *testing.T) {
	dir := setup(t)
	resp, err := replace(req("replace", map[string]any{"path": ".", "pattern": "foo", "replacement": "bar", "apply": true}, dir, false))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Phase != "verify" || resp.Changed == 0 {
		t.Fatalf("apply should change files, got %+v", resp)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if contains(string(data), "foo") {
		t.Fatal("replace should have rewritten foo -> bar")
	}
}

func TestReplaceReadOnlyDenied(t *testing.T) {
	dir := setup(t)
	resp, _ := replace(req("replace", map[string]any{"path": ".", "pattern": "foo", "replacement": "bar", "apply": true}, dir, true))
	if resp.Status != "rejected" {
		t.Fatalf("read-only replace should be rejected, got %s", resp.Status)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
