package batch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oc-ops/oc-ops/internal/contract"
	"github.com/oc-ops/oc-ops/internal/store"
)

func init() {
	_ = os.RemoveAll(store.Root())
}

func setup(t *testing.T) string {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sc"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "sc", "b.txt"), []byte("world"), 0o644)
	return dir
}

func req(args map[string]any, cwd string) contract.Request {
	return contract.Request{Tool: "batch", Args: args, Cwd: cwd}
}

func planID(t *testing.T, dir, id string, args map[string]any) string {
	t.Helper()
	resp, err := Handler(req(args, dir))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("plan status: %s %v", resp.Status, resp.Error)
	}
	if id != "" {
		return id
	}
	var m map[string]any
	_ = json.Unmarshal(resp.Items[0], &m)
	return m["id"].(string)
}

func TestPlanIsNoEffect(t *testing.T) {
	dir := setup(t)
	pid := planID(t, dir, "", map[string]any{
		"action": "plan",
		"operations": []any{
			map[string]any{"op": "copy", "path": "a.txt", "to": "sc/a-copy.txt"},
		},
		"owner": "s1",
	})
	if pid == "" {
		t.Fatal("plan should return an id")
	}
	if _, err := os.Stat(filepath.Join(dir, "sc", "a-copy.txt")); !os.IsNotExist(err) {
		t.Fatal("plan must not have effects")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal("source must remain after plan")
	}
}

func TestPlanResumeExecutes(t *testing.T) {
	dir := setup(t)
	pid := planID(t, dir, "", map[string]any{
		"action": "plan",
		"operations": []any{
			map[string]any{"op": "copy", "path": "a.txt", "to": "sc/a-copy.txt"},
		},
		"owner": "s1",
	})
	resp, err := Handler(req(map[string]any{"action": "resume", "id": pid, "owner": "s1"}, dir))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Changed != 1 {
		t.Fatalf("resume should copy 1 item, got %+v", resp)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sc", "a-copy.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("copied content wrong: %v %q", err, data)
	}
}

func TestRunExecutesImmediately(t *testing.T) {
	dir := setup(t)
	resp, err := Handler(req(map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "copy", "path": "a.txt", "to": "sc/a-copy.txt"}},
		"owner":      "s1",
	}, dir))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Changed != 1 {
		t.Fatalf("run should copy 1 item, got %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(dir, "sc", "a-copy.txt")); err != nil {
		t.Fatal("run should have created the copy")
	}
}

func TestRunRejectsOverwrite(t *testing.T) {
	dir := setup(t)
	// destination already exists
	resp, _ := Handler(req(map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "copy", "path": "a.txt", "to": "sc/b.txt"}},
		"owner":      "s1",
	}, dir))
	if resp.Failed != 1 || resp.Changed != 0 || resp.Complete {
		t.Fatalf("copy onto existing should fail the item, got %+v", resp)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "sc", "b.txt"))
	if string(data) != "world" {
		t.Fatalf("existing destination must be untouched, got %q", data)
	}
}

func TestRunMove(t *testing.T) {
	dir := setup(t)
	resp, err := Handler(req(map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "move", "path": "a.txt", "to": "sc/moved.txt"}},
		"owner":      "s1",
	}, dir))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Changed != 1 {
		t.Fatalf("move should change 1 item, got %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("source should be gone after move")
	}
	if _, err := os.Stat(filepath.Join(dir, "sc", "moved.txt")); err != nil {
		t.Fatal("destination should exist after move")
	}
}

func TestRunRemove(t *testing.T) {
	dir := setup(t)
	resp, err := Handler(req(map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "remove", "path": "sc/b.txt"}},
		"owner":      "s1",
	}, dir))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" || resp.Changed != 1 {
		t.Fatalf("remove should change 1 item, got %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(dir, "sc", "b.txt")); !os.IsNotExist(err) {
		t.Fatal("target should be gone after remove")
	}
}

func TestRunReadOnlyDenied(t *testing.T) {
	dir := setup(t)
	resp, _ := Handler(contract.Request{Tool: "batch", Args: map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "copy", "path": "a.txt", "to": "sc/x.txt"}},
		"owner":      "s1",
	}, Cwd: dir, ReadOnly: true})
	if resp.Status != "rejected" {
		t.Fatalf("read-only run should be rejected, got %s", resp.Status)
	}
}

func TestResumeOwnerIsolation(t *testing.T) {
	dir := setup(t)
	pid := planID(t, dir, "", map[string]any{
		"action":     "plan",
		"operations": []any{map[string]any{"op": "copy", "path": "a.txt", "to": "sc/a-copy.txt"}},
		"owner":      "s1",
	})
	resp, _ := Handler(req(map[string]any{"action": "resume", "id": pid, "owner": "s-other"}, dir))
	if resp.Status != "rejected" {
		t.Fatalf("cross-owner resume should be rejected, got %s", resp.Status)
	}
}

func TestDependentPathsRejected(t *testing.T) {
	dir := setup(t)
	resp, _ := Handler(req(map[string]any{
		"action": "plan",
		"operations": []any{
			map[string]any{"op": "copy", "path": "a.txt", "to": "sc/x.txt"},
			map[string]any{"op": "remove", "path": "a.txt"},
		},
		"owner": "s1",
	}, dir))
	if resp.Status != "error" {
		t.Fatalf("dependent operations should be rejected, got %s", resp.Status)
	}
}

func TestProtectRejected(t *testing.T) {
	dir := setup(t)
	resp, _ := Handler(req(map[string]any{
		"action":     "run",
		"operations": []any{map[string]any{"op": "remove", "path": "sc/b.txt"}},
		"protect":    []any{"sc"},
		"owner":      "s1",
	}, dir))
	if resp.Status != "error" {
		t.Fatalf("protected path operation should be rejected, got %s", resp.Status)
	}
}
