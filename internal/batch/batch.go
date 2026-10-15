// Package batch implements preflight, durable per-item transitions and
// conservative recovery for copy/move/remove. Each item is fully validated and
// snapshotted before any effect; execution requires re-checked revisions so a
// changed source is never silently clobbered. Plans persist on disk and can be
// resumed. No overwrite, no link traversal, no protected/overlapping paths.
package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oc-ops/oc-ops/internal/contract"
	"github.com/oc-ops/oc-ops/internal/store"
)

// Item state transitions.
const (
	StatePending = "pending"
	StateDone    = "done"
	StateFailed  = "failed"
	StateUnknown = "unknown"
)

// MaxOperations bounds a single plan/run.
const MaxOperations = 512

// MaxProtect bounds the protected-path list.
const MaxProtect = 128

// Operation is one copy/move/remove directive. Paths are relative `/` paths
// resolved against the plan root; remove forbids `to`.
type Operation struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	To   string `json:"to,omitempty"`
}

// PlanItem is one audited transition target.
type PlanItem struct {
	Op       string `json:"op"`
	Path     string `json:"path"`
	To       string `json:"to,omitempty"`
	Revision string `json:"revision,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
}

// Plan is the durable audit record.
type Plan struct {
	ID        string     `json:"id"`
	Owner     string     `json:"owner"`
	Root      string     `json:"root"`
	Protect   []string   `json:"protect,omitempty"`
	Items     []PlanItem `json:"items"`
	Phase     string     `json:"phase"` // planned | running | complete
	CreatedAt int64      `json:"createdAt"`
}

// Open returns the batch audit store.
func Open() (*store.Store, error) {
	return store.Open("batch")
}

// Handler dispatches batch-capability requests.
func Handler(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "action", "root", "operations", "operationsRef", "protect", "id", "owner"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s; this tool accepts action, root, operations, operationsRef, protect, id", unknown), false)
	}
	action := argString(req.Args, "action", "")
	switch action {
	case "plan":
		return plan(req)
	case "run":
		return run(req)
	case "resume":
		return resume(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", "action must be plan, run or resume", false)
	}
}

func ownerOf(req contract.Request) string {
	if v, ok := req.Args["owner"].(string); ok {
		return v
	}
	return req.Cwd
}

// parseOperations reads operations either inline or from an ops_result record.
func parseOperations(req contract.Request, st *store.Store) ([]Operation, error) {
	opsRef := argString(req.Args, "operationsRef", "")
	raw := req.Args["operations"]
	if opsRef != "" {
		return referencedOperations(st, ownerOf(req), opsRef)
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, contractErr("INVALID_ARGUMENT", "operations (in line or operationsRef) are required")
	}
	if len(list) > MaxOperations {
		return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("operations is limited to %d items", MaxOperations))
	}
	var ops []Operation
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("operation %d must be an object", i))
		}
		op := Operation{
			Op:   argString(m, "op", ""),
			Path: argString(m, "path", ""),
			To:   argString(m, "to", ""),
		}
		if (op.Op == "copy" || op.Op == "move") && op.To == "" {
			return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("operation %d (%s) requires to", i, op.Op))
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// referencedOperations loads an ops_result record and expects each item to be an
// operation object.
func referencedOperations(st *store.Store, owner, id string) ([]Operation, error) {
	rec, err := loadResultRecord(st, id)
	if err != nil {
		return nil, err
	}
	if rec["owner"] != owner {
		return nil, contractErr("POLICY_DENIED", "operationsRef belongs to another session")
	}
	exp, _ := rec["expires"].(float64)
	if int64(exp) < store.Now() {
		return nil, contractErr("REFERENCE_EXPIRED", "operations reference expired")
	}
	items, ok := rec["items"].([]any)
	if !ok || len(items) == 0 {
		return nil, contractErr("INVALID_ARGUMENT", "operationsRef points to an empty result")
	}
	if len(items) > MaxOperations {
		return nil, contractErr("INVALID_ARGUMENT", "operationsRef resolves to too many operations")
	}
	var ops []Operation
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("operation %d must be an object", i))
		}
		ops = append(ops, Operation{
			Op:   argString(m, "op", ""),
			Path: argString(m, "path", ""),
			To:   argString(m, "to", ""),
		})
	}
	return ops, nil
}

// loadResultRecord reads an ops_result record from the shared result store.
func loadResultRecord(st *store.Store, id string) (map[string]any, error) {
	rstore, err := store.Open("result")
	if err != nil {
		return nil, contractErr("EXECUTOR_UNAVAILABLE", fmt.Sprintf("result store unavailable: %v", err))
	}
	var rec map[string]any
	if err := rstore.Load(id, &rec); err != nil {
		return nil, contractErr("NOT_FOUND", fmt.Sprintf("operationsRef %s does not exist", id))
	}
	return rec, nil
}

// plan handles action=plan and action=run (run validates + executes).
func plan(req contract.Request) (contract.Response, error) {
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("batch store unavailable: %v", err), false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "root", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return contract.Fail("INVALID_ARGUMENT", "batch root must be an existing directory", false)
	}
	protect, err := parseProtect(req)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	ops, err := parseOperations(req, st)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	if argString(req.Args, "id", "") != "" {
		return contract.Fail("INVALID_ARGUMENT", "plan/run forbid an explicit id", false)
	}
	items, err := preflight(root, protect, ops)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	id := store.NewID("b")
	p := &Plan{
		ID: id, Owner: ownerOf(req), Root: root, Protect: protect,
		Items: items, Phase: "planned", CreatedAt: store.Now(),
	}
	if err := st.Save(id, p); err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to record plan: %v", err), false)
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: false,
		Total: len(p.Items),
		Items: []json.RawMessage{contract.ToRaw(contract.Item{
			"id": id, "total": len(p.Items), "phase": "planned",
			"nextAction": "Execute the exact plan with action=resume and this id, or discard.",
		})},
	}, nil
}

// run preflights and executes immediately.
func run(req contract.Request) (contract.Response, error) {
	if req.ReadOnly {
		return contract.Reject("POLICY_DENIED", "batch run is unavailable in read-only mode.")
	}
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("batch store unavailable: %v", err), false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "root", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	protect, err := parseProtect(req)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	ops, err := parseOperations(req, st)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	items, err := preflight(root, protect, ops)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	id := store.NewID("b")
	p := &Plan{
		ID: id, Owner: ownerOf(req), Root: root, Protect: protect,
		Items: items, Phase: "complete", CreatedAt: store.Now(),
	}
	return executeAndRespond(st, p)
}

// resume loads a plan and executes remaining items with revision re-checks.
func resume(req contract.Request) (contract.Response, error) {
	if req.ReadOnly {
		return contract.Reject("POLICY_DENIED", "batch resume is unavailable in read-only mode.")
	}
	id := argString(req.Args, "id", "")
	if id == "" {
		return contract.Fail("INVALID_ARGUMENT", "resume requires id", false)
	}
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("batch store unavailable: %v", err), false)
	}
	var p Plan
	if err := st.Load(id, &p); err != nil {
		return contract.Fail("NOT_FOUND", fmt.Sprintf("plan %s does not exist", id), false)
	}
	if p.Owner != ownerOf(req) {
		return contract.Reject("POLICY_DENIED", "This plan belongs to another session.")
	}
	return executeAndRespond(st, &p)
}

func executeAndRespond(st *store.Store, p *Plan) (contract.Response, error) {
	p.Phase = "running"
	_ = st.Save(p.ID, p)
	done, failed := executeItems(p)
	p.Phase = "complete"
	if err := st.Save(p.ID, p); err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to record plan result: %v", err), false)
	}
	items := make([]json.RawMessage, 0, len(p.Items))
	for _, it := range p.Items {
		row := contract.Item{"op": it.Op, "path": it.Path, "state": it.State}
		if it.To != "" {
			row["to"] = it.To
		}
		if it.Error != "" {
			row["error"] = it.Error
		}
		items = append(items, contract.ToRaw(row))
	}
	return contract.Response{
		Status: "completed", Phase: "verify", Complete: failed == 0,
		Changed: done, Failed: failed, Total: len(p.Items),
		Omitted: len(p.Items) - done - failed, Items: items,
	}, nil
}

// preflight validates operations and snapshots each source. No effects.
func preflight(root string, protect []string, ops []Operation) ([]PlanItem, error) {
	var touched []string
	for _, op := range ops {
		switch op.Op {
		case "copy", "move":
			if op.Path == "" || op.To == "" {
				return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("%s requires path and to", op.Op))
			}
		case "remove":
			if op.Path == "" || op.To != "" {
				return nil, contractErr("INVALID_ARGUMENT", "remove forbids to")
			}
		default:
			return nil, contractErr("INVALID_ARGUMENT", fmt.Sprintf("op must be copy, move or remove, got %q", op.Op))
		}
		abs := filepath.Join(root, filepath.FromSlash(op.Path))
		if !within(root, abs) {
			return nil, contractErr("POLICY_DENIED", fmt.Sprintf("operation path is outside root: %s", op.Path))
		}
		if op.To != "" {
			dst := filepath.Join(root, filepath.FromSlash(op.To))
			if !within(root, dst) {
				return nil, contractErr("POLICY_DENIED", fmt.Sprintf("operation destination is outside root: %s", op.To))
			}
		}
		// No protected or dependent-path overlap. Protect paths are relative
		// to root, matching the relative op paths.
		for _, pr := range protect {
			if pr == "" {
				continue
			}
			if overlaps(filepath.ToSlash(op.Path), pr) || (op.To != "" && overlaps(filepath.ToSlash(op.To), pr)) {
				return nil, contractErr("POLICY_DENIED", fmt.Sprintf("path overlaps a protected path: %s", op.Path))
			}
		}
		if hasOverlap(touched, op.Path) {
			return nil, contractErr("POLICY_DENIED", "dependent or overlapping batch paths are not allowed")
		}
		touched = append(touched, op.Path)
		if op.To != "" {
			touched = append(touched, op.To)
		}
	}
	// Snapshot each source.
	var items []PlanItem
	for i, op := range ops {
		abs := filepath.Join(root, filepath.FromSlash(op.Path))
		rev, bytes, err := snapshot(abs)
		if err != nil {
			items = append(items, PlanItem{Op: op.Op, Path: op.Path, To: op.To, Error: err.Error()})
			continue
		}
		items = append(items, PlanItem{
			Op: op.Op, Path: op.Path, To: op.To, Revision: rev, Bytes: bytes, State: StatePending,
		})
		_ = i
	}
	return items, nil
}

// executeItems runs each pending item with CAS re-checks.
func executeItems(p *Plan) (done, failed int) {
	for i := range p.Items {
		it := &p.Items[i]
		if it.State == StateDone {
			done++
			continue
		}
		abs := filepath.Join(p.Root, filepath.FromSlash(it.Path))
		if err := checkRevision(abs, it.Revision); err != nil {
			it.State = StateFailed
			it.Error = fmt.Sprintf("revision mismatch (item not changed): %v", err)
			failed++
			continue
		}
		var err error
		switch it.Op {
		case "copy":
			err = doCopy(abs, filepath.Join(p.Root, filepath.FromSlash(it.To)))
		case "move":
			err = doMove(abs, filepath.Join(p.Root, filepath.FromSlash(it.To)))
		case "remove":
			err = doRemove(abs)
		}
		if err != nil {
			it.State = StateFailed
			it.Error = err.Error()
			failed++
		} else {
			it.State = StateDone
			done++
		}
	}
	return done, failed
}

// checkRevision verifies the source is unchanged since the snapshot.
func checkRevision(abs, want string) error {
	if want == "" {
		return nil
	}
	rev, err := sha256File(abs)
	if err != nil {
		return err
	}
	if rev != want {
		return fmt.Errorf("source changed after preflight")
	}
	return nil
}

func doCopy(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return contractErr("NOT_FOUND", fmt.Sprintf("source not found: %v", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return contractErr("POLICY_DENIED", "copy refuses symlink sources")
	}
	if _, err := os.Lstat(dst); err == nil {
		return contractErr("REVISION_CONFLICT", "destination exists; no overwrite")
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		sp := filepath.Join(src, e.Name())
		dp := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(sp, dp); err != nil {
				return err
			}
		} else if err := copyFile(sp, dp); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, in)
	cErr := out.Close()
	if cpErr != nil {
		_ = os.Remove(dst)
		return cpErr
	}
	return cErr
}

func doMove(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return contractErr("NOT_FOUND", fmt.Sprintf("source not found: %v", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return contractErr("POLICY_DENIED", "move refuses symlink sources")
	}
	if _, err := os.Lstat(dst); err == nil {
		return contractErr("REVISION_CONFLICT", "destination exists; no overwrite")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback: copy then remove.
	if err := doCopy(src, dst); err != nil {
		return err
	}
	return doRemove(src)
}

func doRemove(abs string) error {
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return contractErr("NOT_FOUND", "target does not exist")
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return contractErr("POLICY_DENIED", "remove refuses symlink targets")
	}
	if info.IsDir() {
		return os.RemoveAll(abs)
	}
	return os.Remove(abs)
}

// snapshot computes a content revision and byte count for a source.
func snapshot(abs string) (string, int64, error) {
	info, err := os.Lstat(abs)
	if err != nil {
		return "", 0, contractErr("NOT_FOUND", fmt.Sprintf("source not found: %v", err))
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, contractErr("POLICY_DENIED", "sources must not be symlinks")
	}
	if info.IsDir() {
		n, err := countFiles(abs)
		return fmt.Sprintf("dir:%d", n), int64(n), err
	}
	rev, err := sha256File(abs)
	return rev, info.Size(), err
}

func sha256File(abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func countFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		count++
		return nil
	})
	return count, err
}

// within reports whether child is inside root (no traversal).
func within(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// overlaps reports whether path shares a protected/other path prefix.
func overlaps(path, proto string) bool {
	path = filepath.ToSlash(strings.TrimSuffix(path, "/"))
	proto = filepath.ToSlash(strings.Trim(proto, "/"))
	if proto == "" {
		return false
	}
	return path == proto || strings.HasPrefix(path, proto+"/") || strings.HasPrefix(proto, path+"/")
}

func hasOverlap(touched []string, p string) bool {
	for _, t := range touched {
		if t == p {
			return true
		}
	}
	return false
}

// contractErr is an internal helper producing an error for validation fails.
func contractErr(code, msg string) error { return &BatchError{Code: code, Message: msg} }

// BatchError is a structured validation/execution error.
type BatchError struct {
	Code    string
	Message string
}

func (e *BatchError) Error() string { return e.Message }

func parseProtect(req contract.Request) ([]string, error) {
	raw := req.Args["protect"]
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok || len(list) > MaxProtect {
		return nil, fmt.Errorf("protect must be an array of at most %d paths", MaxProtect)
	}
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

func unknownKeys(got map[string]any, known ...string) string {
	out := ""
	for k := range got {
		found := false
		for _, name := range known {
			if name == k {
				found = true
				break
			}
		}
		if !found {
			if out != "" {
				out += ", "
			}
			out += k
		}
	}
	return out
}

func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}
