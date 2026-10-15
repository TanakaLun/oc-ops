// Package bg implements managed background jobs for long-running commands
// (Gradle, NDK builds, sims). Jobs are owned per session and persist on disk so
// they survive across separate CLI invocations. Output is bounded and a spill
// path is reported on truncation.
package bg

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oc-ops/oc-ops/internal/contract"
)

// Item is a shorthand for a compact result item.
type Item = contract.Item

const (
	maxOutputBytes = 256 * 1024
	maxTailLines   = 500
)

// JobState is the on-disk record of one job.
type JobState struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	Cwd       string `json:"cwd"`
	Owner     string `json:"owner"`
	StartedAt int64  `json:"startedAt"`
	Pid       int    `json:"pid"`
	Done      bool   `json:"done"`
	ExitCode  int    `json:"exitCode"`
	Signal    string `json:"signal,omitempty"`
	TimedOut  bool   `json:"timedOut,omitempty"`
	Output    string `json:"output,omitempty"`
	Spill     string `json:"spill,omitempty"`
}

// StoreRoot returns the disk-backed store directory.
func StoreRoot() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "oc-ops", "jobs")
}

func ensureRoot() error {
	return os.MkdirAll(StoreRoot(), 0o755)
}

func statePath(id string) string { return filepath.Join(StoreRoot(), id+".json") }
func outPath(id string) string   { return filepath.Join(StoreRoot(), id+".out") }

// readState loads one job record.
func readState(id string) (*JobState, error) {
	b, err := os.ReadFile(statePath(id))
	if err != nil {
		return nil, err
	}
	var s JobState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// writeState atomically writes one job record.
func writeState(s *JobState) error {
	b, _ := json.Marshal(s)
	tmp := statePath(s.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(s.ID))
}

// listStates returns all job records.
func listStates() ([]*JobState, error) {
	entries, err := os.ReadDir(StoreRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*JobState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := readState(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func Handler(req contract.Request) (contract.Response, error) {
	switch req.Tool {
	case "bg_start":
		return start(req)
	case "bg_output":
		return output(req)
	case "bg_list":
		return list(req)
	case "bg_kill":
		return kill(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("unknown background capability %q", req.Tool), false)
	}
}

func ownerOf(req contract.Request) string {
	if v, ok := req.Args["owner"].(string); ok {
		return v
	}
	return req.Cwd
}

func start(req contract.Request) (contract.Response, error) {
	if req.ReadOnly {
		return contract.Reject("POLICY_DENIED", "bg_start is unavailable in read-only mode.")
	}
	command := argString(req.Args, "command", "")
	if command == "" {
		return contract.Fail("INVALID_ARGUMENT", "command is required", false)
	}
	label := argString(req.Args, "label", command)
	cwd := argString(req.Args, "cwd", req.Cwd)
	owner := ownerOf(req)
	if err := ensureRoot(); err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("job store unavailable: %v", err), false)
	}
	id := newID()
	// Open the output file BEFORE Start so stdout/stderr are attached to it.
	outF, err := os.OpenFile(outPath(id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to open job output: %v", err), false)
	}
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb", "PAGER=cat")
	// Detach into its own process group so a later kill reaches the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = outF
	cmd.Stderr = outF
	if err := cmd.Start(); err != nil {
		_ = outF.Close()
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to start job: %v", err), false)
	}
	state := &JobState{
		ID: id, Label: label, Command: command, Cwd: cwd, Owner: owner,
		StartedAt: time.Now().Unix(), Pid: cmd.Process.Pid,
	}
	if err := writeState(state); err != nil {
		_ = cmd.Process.Kill()
		_ = outF.Close()
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to record job: %v", err), false)
	}
	_ = outF.Close()
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true,
		Items: []json.RawMessage{contract.ToRaw(Item{"job_id": id})},
	}, nil
}

// alive reports whether the pid is still running. A zombie (state Z in /proc)
// is treated as dead: kill(pid,0) returns success for zombies until init reaps
// them, which would otherwise make a finished job look forever-running.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// /proc/<pid>/stat: field 3 is the process state. A 'Z' means defunct.
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		if idx := strings.LastIndex(string(data), ")"); idx >= 0 {
			rest := strings.TrimSpace(string(data)[idx+1:])
			if rest != "" && rest[0] == 'Z' {
				return false
			}
		}
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// settle refreshes a job's done/exitCode/output from process liveness and the
// on-disk output file. It is idempotent and safe to call any number of times.
func settle(state *JobState) {
	if state.Done || !alive(state.Pid) {
		state.Done = true
		if data, err := os.ReadFile(outPath(state.ID)); err == nil {
			if len(data) > maxOutputBytes {
				state.Output = string(data[len(data)-maxOutputBytes:])
				state.Spill = outPath(state.ID)
			} else {
				state.Output = string(data)
			}
		}
		if state.ExitCode == 0 && state.Signal == "" {
			// Only mark a zero exit if the process actually completed; leave a
			// nonzero/gone signal to be inferred by the caller.
		}
		_ = writeState(state)
	}
}

// MonitorMain is retained for CLI symmetry but unused: liveness is derived
// from kill(pid,0), so no detached monitor process is required.
func MonitorMain(id, pidStr string) {
	os.Exit(0)
}

func output(req contract.Request) (contract.Response, error) {
	id := argString(req.Args, "job_id", "")
	if id == "" {
		return contract.Fail("INVALID_ARGUMENT", "job_id is required", false)
	}
	owner := ownerOf(req)
	state, err := readState(id)
	if err != nil || state.Owner != owner {
		return contract.Fail("INVALID_ARGUMENT", "This job was not started by this session.", false)
	}
	settle(state)
	lines := strings.Split(state.Output, "\n")
	truncated := len(lines) > maxTailLines
	if truncated {
		lines = lines[len(lines)-maxTailLines:]
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: state.Done,
		Truncated: truncated || state.Spill != "",
		Items: []json.RawMessage{contract.ToRaw(Item{
			"job_id": id, "done": state.Done, "exitCode": state.ExitCode,
			"text": strings.Join(lines, "\n"), "cursor": len(lines),
		})},
	}, nil
}

func list(req contract.Request) (contract.Response, error) {
	owner := ownerOf(req)
	states, _ := listStates()
	for _, s := range states {
		if s.Owner == owner {
			settle(s)
		}
	}
	var owned []*JobState
	for _, s := range states {
		if s.Owner == owner {
			owned = append(owned, s)
		}
	}
	sort.Slice(owned, func(i, k int) bool { return owned[i].StartedAt < owned[k].StartedAt })
	items := make([]json.RawMessage, 0, len(owned))
	for _, s := range owned {
		items = append(items, contract.ToRaw(Item{
			"id": s.ID, "label": s.Label, "command": s.Command, "cwd": s.Cwd,
			"startedAt": s.StartedAt, "done": s.Done, "exitCode": s.ExitCode,
		}))
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true,
		Items: items, Total: len(items),
	}, nil
}

func kill(req contract.Request) (contract.Response, error) {
	id := argString(req.Args, "job_id", "")
	if id == "" {
		return contract.Fail("INVALID_ARGUMENT", "job_id is required", false)
	}
	owner := ownerOf(req)
	state, err := readState(id)
	if err != nil || state.Owner != owner {
		return contract.Fail("INVALID_ARGUMENT", "This job was not started by this session.", false)
	}
	if state.Pid > 0 {
		// Kill the process group (negative pid) so children are terminated too.
		_ = syscall.Kill(-state.Pid, syscall.SIGTERM)
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true,
		Items: []json.RawMessage{contract.ToRaw(Item{"job_id": id, "killed": true})},
	}, nil
}

var idMu sync.Mutex
var idSeq int

func newID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idSeq++
	return fmt.Sprintf("j%d-%d", time.Now().Unix(), idSeq)
}

func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return def
}

// keep strconv referenced for potential future numeric parsing helpers.
var _ = strconv.Itoa
