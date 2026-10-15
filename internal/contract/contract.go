// Package contract defines the stable JSON envelope shared by every oc-ops
// backend subcommand and the opencode plugin. The backend is a pure function
// of (subcommand, args JSON, stdin, cwd, env): it never holds session state.
package contract

import (
	"encoding/json"
	"time"
)

// Request is the normalized input every handler receives.
type Request struct {
	// Tool is the raw subcommand name (grep, glob, inspect, replace, map,
	// bg_start, ...) — NOT the ops_ public name.
	Tool string `json:"tool"`
	// Args is the model-facing argument object, already validated against the
	// tool schema by the plugin. Handlers re-validate defensively.
	Args map[string]any `json:"args"`
	// Cwd is the session working directory, used to resolve relative paths.
	Cwd string `json:"cwd,omitempty"`
	// ReadOnly disallows any mutation (replace, bg_start). Set by the plugin
	// when the session is read-only.
	ReadOnly bool `json:"read_only,omitempty"`
	// Deadline, when set, bounds the whole call.
	Deadline time.Time `json:"-"`
}

// Response is the canonical envelope every handler returns.
type Response struct {
	Status     string            `json:"status"` // completed | rejected | error
	Phase      string            `json:"phase"`  // observe | verify
	Changed    int               `json:"changed"`
	Matched    int               `json:"matched"`
	Skipped    int               `json:"skipped"`
	Failed     int               `json:"failed"`
	Complete   bool              `json:"complete"`
	Truncated  bool              `json:"truncated"`
	Omitted    int               `json:"omitted"`
	Total      int               `json:"total"`
	NextOffset int               `json:"nextOffset"`
	Items      []json.RawMessage `json:"items,omitempty"`
	Error      *Error            `json:"error,omitempty"`
}

// Error is a structured, actionable failure.
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"nextAction,omitempty"`
}

// Item is a convenience for building compact Items entries.
type Item map[string]any

// Handler is the interface every backend capability implements.
type Handler func(req Request) (Response, error)

// OK returns a completed response with the given items.
func OK(items []json.RawMessage, matched, total, offset int, truncated bool) Response {
	return Response{
		Status:     "completed",
		Phase:      "verify",
		Matched:    matched,
		Total:      total,
		Omitted:    max(0, total-(offset+len(items))),
		NextOffset: offset + len(items),
		Truncated:  truncated,
		Complete:   !truncated && offset+len(items) >= total,
		Items:      items,
	}
}

// Reject returns a completed-but-rejected response (policy denial).
func Reject(code, message string) (Response, error) {
	return Response{
		Status: "rejected",
		Phase:  "preflight",
		Error:  &Error{Code: code, Message: message, Retryable: false},
	}, nil
}

// Fail returns an error response.
func Fail(code, message string, retryable bool) (Response, error) {
	return Response{
		Status: "error",
		Phase:  "preflight",
		Error:  &Error{Code: code, Message: message, Retryable: retryable},
	}, nil
}

// Encode marshals a response. nil items becomes an empty array.
func (r Response) Encode() ([]byte, error) {
	if r.Items == nil {
		r.Items = []json.RawMessage{}
	}
	return json.Marshal(r)
}

// ToRaw marshals one item into a RawMessage, for use in Items.
func ToRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Paged slices already-built items by offset/limit and returns the response.
func Paged(items []json.RawMessage, offset, limit int, truncated bool) Response {
	total := len(items)
	start := offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return OK(items[start:end], total, total, offset, truncated)
}
