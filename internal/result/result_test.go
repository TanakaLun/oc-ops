package result

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

func mustPut(t *testing.T, items []any) string {
	t.Helper()
	resp, err := Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "put", "items": items, "owner": "sess-1",
	}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("put status: %+v", resp)
	}
	var m map[string]any
	_ = json.Unmarshal(resp.Items[0], &m)
	return m["id"].(string)
}

func TestResultPutPageRelease(t *testing.T) {
	dir := t.TempDir()
	id := mustPut(t, []any{
		map[string]any{"name": "a", "v": 1},
		map[string]any{"name": "b", "v": 2},
		map[string]any{"name": "a", "v": 9},
	})

	// page with equals filter
	resp, err := Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "equals": map[string]any{"name": "a"}, "owner": "sess-1",
	}, Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected 2 rows matching name=a, got total %d", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	// fields projection
	resp, _ = Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "fields": []any{"name"}, "owner": "sess-1",
	}, Cwd: dir})
	for _, raw := range resp.Items {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if _, has := m["name"]; !has {
			t.Fatal("projection should include name")
		}
		if _, has := m["v"]; has {
			t.Fatal("projection should exclude v")
		}
	}

	// byte budget truncation
	resp, _ = Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "maxBytes": 512, "limit": 1, "owner": "sess-1",
	}, Cwd: dir})
	if len(resp.Items) > 1 {
		t.Fatalf("limit 1 should bound items, got %d", len(resp.Items))
	}

	// release
	resp, err = Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "release", "id": id, "owner": "sess-1",
	}, Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("release status: %+v", resp)
	}
	// paging a released record fails
	resp, _ = Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "owner": "sess-1",
	}, Cwd: dir})
	if resp.Status != "error" {
		t.Fatalf("page after release should error, got %s", resp.Status)
	}
}

func TestResultOwnerIsolation(t *testing.T) {
	id := mustPut(t, []any{map[string]any{"x": 1}})
	dir := t.TempDir()
	// other owner cannot page
	resp, _ := Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "owner": "sess-other",
	}, Cwd: dir})
	if resp.Status != "rejected" {
		t.Fatalf("cross-owner page should be rejected, got %s", resp.Status)
	}
	// own owner can page
	resp, _ = Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "page", "id": id, "owner": "sess-1",
	}, Cwd: dir})
	if resp.Status != "completed" {
		t.Fatalf("own page should succeed, got %s", resp.Status)
	}
}

func TestResultPutReadOnly(t *testing.T) {
	dir := t.TempDir()
	resp, _ := Handler(contract.Request{Tool: "result", Args: map[string]any{
		"action": "put", "items": []any{map[string]any{"x": 1}}, "owner": "s",
	}, Cwd: dir, ReadOnly: true})
	if resp.Status != "rejected" {
		t.Fatalf("read-only put should be rejected, got %s", resp.Status)
	}
}

var _ = filepath.Join
