// Package result implements bounded, session-owned result references:
// put object rows, page with top-level equals/fields filters and quantity/byte
// budgets, release results. 24h TTL. References are observations, never
// authority.
package result

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/oc-ops/oc-ops/internal/contract"
	"github.com/oc-ops/oc-ops/internal/store"
)

// TTL is the record lifetime in seconds.
const TTL int64 = 86400

// MaxItems per put.
const MaxItems = 10000

// Record is one stored result reference.
type Record struct {
	ID      string            `json:"id"`
	Owner   string            `json:"owner"`
	Expires int64             `json:"expires"`
	Items   []json.RawMessage `json:"items"`
}

// Fixed ids and prefixes.
// r- ids are result references; op- ids are operation references (see batch).
func validIDPrefix(id, prefix string) bool {
	return len(id) > len(prefix) && id[:len(prefix)] == prefix
}

// Open returns the result store; the caller owns all scope/tTL checks.
func Open() (*store.Store, error) {
	return store.Open("result")
}

// Handler dispatches result-capability requests.
func Handler(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "action", "root", "id", "items", "offset", "limit", "maxBytes", "fields", "equals", "owner", "cwd", "read_only"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s; this tool accepts action, root, id, items, offset, limit, maxBytes, fields, equals", unknown), false)
	}
	action := argString(req.Args, "action", "")
	switch action {
	case "put":
		return put(req)
	case "page":
		return page(req)
	case "release":
		return release(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", "action must be put, page or release", false)
	}
}

// ownerOf is the session identity the plugin stamps on every call.
func ownerOf(req contract.Request) string {
	if v, ok := req.Args["owner"].(string); ok {
		return v
	}
	return req.Cwd
}

func put(req contract.Request) (contract.Response, error) {
	if req.ReadOnly {
		return contract.Reject("POLICY_DENIED", "result put is unavailable in read-only mode.")
	}
	rawItems := req.Args["items"]
	if rawItems == nil {
		return contract.Fail("INVALID_ARGUMENT", "put requires items", false)
	}
	list, ok := rawItems.([]any)
	if !ok || len(list) == 0 || len(list) > MaxItems {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("put requires 1..%d items", MaxItems), false)
	}
	owner := ownerOf(req)
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("result store unavailable: %v", err), false)
	}
	id := store.NewID("r")
	rec := &Record{
		ID:      id,
		Owner:   owner,
		Expires: store.Now() + TTL,
	}
	for _, item := range list {
		if _, ok := item.(map[string]any); !ok {
			return contract.Fail("INVALID_ARGUMENT", "items must be JSON objects", false)
		}
		b, err := json.Marshal(item)
		if err != nil {
			return contract.Fail("INVALID_ARGUMENT", "items must be JSON objects", false)
		}
		rec.Items = append(rec.Items, b)
	}
	if err := st.Save(id, rec); err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("failed to store result: %v", err), false)
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: false, Total: len(rec.Items),
		Items: []json.RawMessage{contract.ToRaw(contract.Item{
			"id": id, "total": len(rec.Items), "expires": rec.Expires,
			"nextAction": "Use ops_result page with this id; the reference grants no filesystem authority.",
		})},
	}, nil
}

func page(req contract.Request) (contract.Response, error) {
	id := argString(req.Args, "id", "")
	if id == "" {
		return contract.Fail("INVALID_ARGUMENT", "page requires id", false)
	}
	owner := ownerOf(req)
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("result store unavailable: %v", err), false)
	}
	var rec Record
	if err := st.Load(id, &rec); err != nil {
		if os.IsNotExist(err) {
			return contract.Fail("NOT_FOUND", fmt.Sprintf("result reference %s does not exist or expired", id), false)
		}
		return contract.Fail("RESULT_UNKNOWN", fmt.Sprintf("failed to read result: %v", err), true)
	}
	if rec.Owner != owner {
		return contract.Reject("POLICY_DENIED", "This result reference belongs to another session.")
	}
	if rec.Expires < store.Now() {
		return contract.Fail("REFERENCE_EXPIRED", fmt.Sprintf("result reference %s expired", id), false)
	}
	offset := argInt(req.Args, "offset", 0)
	limit := argInt(req.Args, "limit", 100)
	maxBytes := argInt(req.Args, "maxBytes", 65536)
	project := argStrings(req.Args, "fields", nil)
	equals := eqFilter(req.Args["equals"])
	if offset < 0 || limit < 1 || limit > 512 {
		return contract.Fail("INVALID_ARGUMENT", "offset must be >= 0 and limit 1..512", false)
	}
	if maxBytes < 512 || maxBytes > 65536 {
		return contract.Fail("INVALID_ARGUMENT", "maxBytes must be 512..65536", false)
	}

	var selected []json.RawMessage
	for _, b := range rec.Items {
		var obj map[string]any
		_ = json.Unmarshal(b, &obj)
		if !matches(obj, equals) {
			continue
		}
		selected = append(selected, projectItem(b, obj, project))
	}
	total := len(selected)
	truncated := false
	omitted := 0
	var emitted []json.RawMessage
	byteBudget := 0
	for i := offset; i < total; i++ {
		if len(emitted) >= limit || byteBudget+len(selected[i]) > maxBytes {
			truncated = true
			if len(emitted) < offset+limit && byteBudget+len(selected[i]) > maxBytes {
				omitted = total - i
			}
			break
		}
		byteBudget += len(selected[i])
		emitted = append(emitted, selected[i])
	}
	if len(emitted) < limit && offset+len(emitted) < total && !truncated {
		truncated = true
	}
	omitted = total - (offset + len(emitted))
	if omitted < 0 {
		omitted = 0
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: !truncated && offset+len(emitted) >= total,
		Truncated: truncated, Total: total, Omitted: omitted,
		NextOffset: offset + len(emitted), Items: emitted,
	}, nil
}

func release(req contract.Request) (contract.Response, error) {
	id := argString(req.Args, "id", "")
	if id == "" {
		return contract.Fail("INVALID_ARGUMENT", "release requires id", false)
	}
	owner := ownerOf(req)
	st, err := Open()
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", fmt.Sprintf("result store unavailable: %v", err), false)
	}
	var rec Record
	if err := st.Load(id, &rec); err != nil {
		if os.IsNotExist(err) {
			return contract.Fail("NOT_FOUND", fmt.Sprintf("result reference %s does not exist", id), false)
		}
		return contract.Fail("RESULT_UNKNOWN", fmt.Sprintf("failed to read result: %v", err), true)
	}
	if rec.Owner != owner {
		return contract.Reject("POLICY_DENIED", "This result reference belongs to another session.")
	}
	if err := st.Delete(id); err != nil {
		return contract.Fail("RESULT_UNKNOWN", fmt.Sprintf("failed to release result: %v", err), true)
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true,
		Items: []json.RawMessage{contract.ToRaw(contract.Item{"id": id, "released": true})},
	}, nil
}

func matches(obj map[string]any, eq map[string]any) bool {
	for k, want := range eq {
		got, has := obj[k]
		if !has {
			return false
		}
		gotB, _ := json.Marshal(got)
		wantB, _ := json.Marshal(want)
		if string(gotB) != string(wantB) {
			return false
		}
	}
	return true
}

func projectItem(raw json.RawMessage, obj map[string]any, fields []string) json.RawMessage {
	if len(fields) == 0 {
		return raw
	}
	proj := map[string]any{}
	for _, f := range fields {
		if v, has := obj[f]; has {
			proj[f] = v
		}
	}
	b, _ := json.Marshal(proj)
	return b
}

func eqFilter(v any) map[string]any {
	out := map[string]any{}
	if m, ok := v.(map[string]any); ok {
		for k, val := range m {
			out[k] = val
		}
	}
	return out
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

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, e := v.Int64(); e == nil {
			return int(n)
		}
	}
	return def
}

func argStrings(args map[string]any, key string, def []string) []string {
	switch v := args[key].(type) {
	case []any:
		var out []string
		for _, it := range v {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	}
	return def
}
