package bg

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/oc-ops/oc-ops/internal/contract"
)

func init() {
	_ = os.RemoveAll(StoreRoot())
}

func TestBgLifecycle(t *testing.T) {
	owner := "test-session"
	// start
	resp, err := Handler(contract.Request{Tool: "bg_start", Args: map[string]any{
		"command": "echo start; echo end", "owner": owner,
	}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "completed" {
		t.Fatalf("start status: %+v", resp)
	}
	var m map[string]any
	_ = json.Unmarshal(resp.Items[0], &m)
	id, _ := m["job_id"].(string)
	if id == "" {
		t.Fatal("expected job_id")
	}

	// list should show it as owned
	listResp, err := Handler(contract.Request{Tool: "bg_list", Args: map[string]any{"owner": owner}})
	if err != nil {
		t.Fatal(err)
	}
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 owned job, got %d", len(listResp.Items))
	}

	// wait for completion, then output
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, _ := readState(id)
		if state != nil {
			settle(state)
			if state.Done {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish in time")
		}
		time.Sleep(50 * time.Millisecond)
	}
	outResp, err := Handler(contract.Request{Tool: "bg_output", Args: map[string]any{"job_id": id, "owner": owner}})
	if err != nil {
		t.Fatal(err)
	}
	if outResp.Status != "completed" {
		t.Fatalf("output status: %+v", outResp)
	}
	var om map[string]any
	_ = json.Unmarshal(outResp.Items[0], &om)
	if om["done"] != true {
		t.Fatalf("expected done, got %v", om["done"])
	}
}

func TestBgOwnershipIsolation(t *testing.T) {
	resp, err := Handler(contract.Request{Tool: "bg_start", Args: map[string]any{
		"command": "echo hi", "owner": "owner-A",
	}, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(resp.Items[0], &m)
	id, _ := m["job_id"].(string)

	// owner-B must not see it
	listB, _ := Handler(contract.Request{Tool: "bg_list", Args: map[string]any{"owner": "owner-B"}})
	if len(listB.Items) != 0 {
		t.Fatalf("owner-B should see 0 jobs, got %d", len(listB.Items))
	}
	// owner-B cannot read it
	outB, _ := Handler(contract.Request{Tool: "bg_output", Args: map[string]any{"job_id": id, "owner": "owner-B"}})
	if outB.Status != "error" {
		t.Fatalf("owner-B output should error, got %s", outB.Status)
	}
	// owner-A can
	outA, _ := Handler(contract.Request{Tool: "bg_output", Args: map[string]any{"job_id": id, "owner": "owner-A"}})
	if outA.Status != "completed" {
		t.Fatalf("owner-A output should succeed, got %s", outA.Status)
	}
}

func TestBgReadOnlyDenied(t *testing.T) {
	resp, _ := Handler(contract.Request{Tool: "bg_start", Args: map[string]any{
		"command": "echo hi", "owner": "x",
	}, Cwd: t.TempDir(), ReadOnly: true})
	if resp.Status != "rejected" {
		t.Fatalf("read-only bg_start should be rejected, got %s", resp.Status)
	}
}
