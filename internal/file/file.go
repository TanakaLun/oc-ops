// Package file implements the four token-efficient file tools:
// grep, glob, inspect, replace. Output is bounded and summary-first.
package file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/oc-ops/oc-ops/internal/contract"
)

// MaxLines bounds content output per call.
const MaxLines = 10000

// MaxContext bounds grep context lines.
const MaxContext = 32

// Item is a shorthand for a compact result item.
type Item = contract.Item

// unknownKeys returns a comma-joined list of arg keys not in known.
func unknownKeys(got map[string]any, known ...string) string {
	var unknown []string
	for k := range got {
		found := false
		for _, name := range known {
			if name == k {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, k)
		}
	}
	return strings.Join(unknown, ", ")
}

func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func argBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
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

func argStrings(args map[string]any, key string, def []string) []string {
	switch v := args[key].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	}
	return def
}

// Handler dispatches file-capability requests.
func Handler(req contract.Request) (contract.Response, error) {
	switch req.Tool {
	case "grep":
		return grep(req)
	case "glob":
		return glob(req)
	case "inspect":
		return inspect(req)
	case "replace":
		return replace(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("unknown file capability %q", req.Tool), false)
	}
}

// ---------- inspect ----------

func inspect(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "path", "offset", "limit", "view"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s; this tool accepts path, offset, limit, view", unknown), false)
	}
	pathRaw := argString(req.Args, "path", ".")
	view := argString(req.Args, "view", "text")
	if view != "text" && view != "hex" {
		return contract.Fail("INVALID_ARGUMENT", "view must be text or hex", false)
	}
	limit := argInt(req.Args, "limit", 200)
	if limit < 1 || limit > MaxLines {
		return contract.Fail("INVALID_ARGUMENT", "limit must be 1..10000", false)
	}
	offset := argInt(req.Args, "offset", 1)
	if offset < 1 {
		return contract.Fail("INVALID_ARGUMENT", "offset must be >= 1", false)
	}
	abs, err := contract.ResolvePath(req.Cwd, pathRaw)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return contract.Reject("ACCESS_DENIED", fmt.Sprintf("cannot read %q: %v", pathRaw, err))
	}
	if info.IsDir() {
		return contract.Fail("INVALID_ARGUMENT", "path is a directory; use ops_glob or ops_grep", false)
	}
	if info.Size() > 32<<20 {
		return contract.Fail("OUTPUT_LIMIT", "file exceeds the inspect size budget", false)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return contract.Reject("ACCESS_DENIED", fmt.Sprintf("cannot read %q: %v", pathRaw, err))
	}
	if view == "hex" {
		return hexView(abs, data, offset-1, limit)
	}
	return textView(abs, data, offset, limit)
}

func textView(abs string, data []byte, offset, limit int) (contract.Response, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)
	startIdx := offset - 1
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + limit
	if endIdx > total {
		endIdx = total
	}
	items := make([]json.RawMessage, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		items = append(items, contract.ToRaw(Item{
			"path": filepath.ToSlash(abs),
			"line": i + 1,
			"text": lines[i],
		}))
	}
	resp := contract.OK(items, total, offset, total, false)
	resp.Phase = "observe"
	return resp, nil
}

func hexView(abs string, data []byte, byteOffset, limit int) (contract.Response, error) {
	const perRow = 16
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(data) {
		byteOffset = len(data)
	}
	totalRows := (len(data) + perRow - 1) / perRow
	startRow := byteOffset / perRow
	if startRow > totalRows {
		startRow = totalRows
	}
	endRow := startRow + limit
	if endRow > totalRows {
		endRow = totalRows
	}
	items := make([]json.RawMessage, 0, endRow-startRow)
	for r := startRow; r < endRow; r++ {
		begin := r * perRow
		fin := begin + perRow
		if fin > len(data) {
			fin = len(data)
		}
		bytes := data[begin:fin]
		var hexParts, ascii strings.Builder
		for _, b := range bytes {
			fmt.Fprintf(&hexParts, "%02x ", b)
			if b >= 0x20 && b < 0x7f {
				ascii.WriteByte(b)
			} else {
				ascii.WriteByte('.')
			}
		}
		items = append(items, contract.ToRaw(Item{
			"path":   filepath.ToSlash(abs),
			"view":   "hex",
			"offset": begin,
			"hex":    strings.TrimSpace(hexParts.String()),
			"ascii":  ascii.String(),
		}))
	}
	resp := contract.OK(items, totalRows, startRow*perRow+1, totalRows, false)
	resp.Phase = "observe"
	return resp, nil
}

// ---------- glob ----------

func glob(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "path", "pattern", "outputMode", "sort", "offset", "limit"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s; this tool accepts path, pattern, outputMode, sort, offset, limit", unknown), false)
	}
	patterns := argStrings(req.Args, "pattern", nil)
	if len(patterns) == 0 {
		return contract.Fail("INVALID_ARGUMENT", "pattern requires at least one positive glob", false)
	}
	hasPositive := false
	for _, p := range patterns {
		if !strings.HasPrefix(p, "!") {
			hasPositive = true
			break
		}
	}
	if !hasPositive {
		return contract.Fail("INVALID_ARGUMENT", "pattern requires at least one positive (non-!) glob", false)
	}
	outputMode := argString(req.Args, "outputMode", "paths")
	if outputMode != "paths" && outputMode != "details" {
		return contract.Fail("INVALID_ARGUMENT", "outputMode must be paths or details", false)
	}
	sortMode := argString(req.Args, "sort", "path")
	if sortMode != "path" && sortMode != "modified" {
		return contract.Fail("INVALID_ARGUMENT", "sort must be path or modified", false)
	}
	offset := argInt(req.Args, "offset", 0)
	limit := argInt(req.Args, "limit", 100)
	if offset < 0 || limit < 1 || limit > MaxLines {
		return contract.Fail("INVALID_ARGUMENT", "offset must be >= 0 and limit 1..10000", false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "path", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return contract.Fail("INVALID_ARGUMENT", "glob path must be an existing directory", false)
	}
	var include, exclude []string
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			exclude = append(exclude, strings.TrimPrefix(p, "!"))
		} else {
			include = append(include, p)
		}
	}
	type fileInfo struct {
		path     string
		modified int64
		size     int64
	}
	var files []fileInfo
	contract.Walk(contract.WalkOption{
		Root:        root,
		MaxDepth:    64,
		MaxEntries:  100000,
		IncludeGlob: include,
		ExcludeGlob: exclude,
		SkipHidden:  true,
	}, func(abs, rel string) bool {
		mi, err := os.Stat(abs)
		if err != nil {
			return true
		}
		files = append(files, fileInfo{path: filepath.ToSlash(abs), modified: mi.ModTime().Unix(), size: mi.Size()})
		return true
	})
	if sortMode == "modified" {
		sort.Slice(files, func(i, j int) bool { return files[i].modified > files[j].modified })
	} else {
		sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	}
	total := len(files)
	startIdx := offset
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + limit
	if endIdx > total {
		endIdx = total
	}
	items := make([]json.RawMessage, 0, endIdx-startIdx)
	for _, f := range files[startIdx:endIdx] {
		if outputMode == "paths" {
			items = append(items, contract.ToRaw(f.path))
		} else {
			items = append(items, contract.ToRaw(Item{
				"path":     f.path,
				"bytes":    f.size,
				"modified": f.modified,
			}))
		}
	}
	return contract.OK(items, total, offset, total, false), nil
}

// ---------- grep ----------

func grep(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "pattern", "path", "glob", "outputMode", "caseInsensitive", "lineNumbers", "beforeContext", "afterContext", "multiline", "offset", "limit"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s", unknown), false)
	}
	pattern := argString(req.Args, "pattern", "")
	if pattern == "" {
		return contract.Fail("INVALID_ARGUMENT", "pattern is required", false)
	}
	outputMode := argString(req.Args, "outputMode", "summary")
	switch outputMode {
	case "content", "files_with_matches", "count", "summary":
	default:
		return contract.Fail("INVALID_ARGUMENT", "outputMode must be content, files_with_matches, count or summary", false)
	}
	ci := argBool(req.Args, "caseInsensitive", false)
	lineNos := argBool(req.Args, "lineNumbers", true)
	before := argInt(req.Args, "beforeContext", 0)
	after := argInt(req.Args, "afterContext", 0)
	if before < 0 || before > MaxContext || after < 0 || after > MaxContext {
		return contract.Fail("INVALID_ARGUMENT", "context is limited to 32 lines", false)
	}
	offset := argInt(req.Args, "offset", 0)
	limit := argInt(req.Args, "limit", 100)
	if offset < 0 || limit < 1 || limit > MaxLines {
		return contract.Fail("INVALID_ARGUMENT", "offset must be >= 0 and limit 1..10000", false)
	}
	flags := ""
	if ci {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid pattern: %v", err), false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "path", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return contract.Fail("INVALID_ARGUMENT", "grep path must be an existing directory", false)
	}
	var include, exclude []string
	for _, g := range argStrings(req.Args, "glob", nil) {
		if strings.HasPrefix(g, "!") {
			exclude = append(exclude, strings.TrimPrefix(g, "!"))
		} else {
			include = append(include, g)
		}
	}
	var items []json.RawMessage
	var matched, fileCount int
	walkTruncated := contract.Walk(contract.WalkOption{
		Root:        root,
		MaxDepth:    64,
		MaxEntries:  100000,
		IncludeGlob: include,
		ExcludeGlob: exclude,
		SkipHidden:  true,
	}, func(abs, rel string) bool {
		data, err := os.ReadFile(abs)
		if err != nil || !utf8.Valid(data) {
			return true
		}
		lines := strings.Split(string(data), "\n")
		var lineHits []int
		for i, line := range lines {
			if re.MatchString(line) {
				lineHits = append(lineHits, i)
			}
		}
		if len(lineHits) == 0 {
			return true
		}
		fileCount++
		matched += len(lineHits)
		switch outputMode {
		case "files_with_matches":
			items = append(items, contract.ToRaw(Item{"path": filepath.ToSlash(abs)}))
		case "count":
			items = append(items, contract.ToRaw(Item{"path": filepath.ToSlash(abs), "count": len(lineHits)}))
		default: // content
			for _, i := range lineHits {
				start := i - before
				if start < 0 {
					start = 0
				}
				end := i + after + 1
				if end > len(lines) {
					end = len(lines)
				}
				block := strings.Join(lines[start:end], "\n")
				ln := 0
				if lineNos {
					ln = i + 1
				}
				items = append(items, contract.ToRaw(Item{
					"path": filepath.ToSlash(abs),
					"line": ln,
					"text": block,
				}))
			}
		}
		if outputMode != "summary" && len(items) >= offset+limit {
			return false
		}
		return true
	})
	if outputMode == "summary" {
		return contract.Response{
			Status:    "completed",
			Phase:     "observe",
			Matched:   matched,
			Total:     matched,
			Complete:  !walkTruncated,
			Truncated: walkTruncated,
			Items: []json.RawMessage{
				contract.ToRaw(Item{"files": fileCount, "matches": matched}),
			},
		}, nil
	}
	total := len(items)
	startIdx := offset
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + limit
	if endIdx > total {
		endIdx = total
	}
	resp := contract.OK(items[startIdx:endIdx], matched, total, offset, walkTruncated)
	resp.Phase = "observe"
	return resp, nil
}

// ---------- replace ----------

func replace(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "path", "pattern", "replacement", "glob", "maxReplacements", "apply", "dryRun"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s", unknown), false)
	}
	pattern := argString(req.Args, "pattern", "")
	replacement := argString(req.Args, "replacement", "")
	if pattern == "" {
		return contract.Fail("INVALID_ARGUMENT", "pattern is required", false)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid pattern: %v", err), false)
	}
	apply := argBool(req.Args, "apply", false)
	dryRun := argBool(req.Args, "dryRun", false)
	writeMode := apply && !dryRun
	if writeMode && req.ReadOnly {
		return contract.Reject("POLICY_DENIED", "replace is unavailable in read-only mode; no files were changed.")
	}
	maxReplacements := argInt(req.Args, "maxReplacements", 1000000)
	if maxReplacements < 1 || maxReplacements > 1000000 {
		return contract.Fail("INVALID_ARGUMENT", "maxReplacements must be 1..1000000", false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "path", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	var include, exclude []string
	for _, g := range argStrings(req.Args, "glob", nil) {
		if strings.HasPrefix(g, "!") {
			exclude = append(exclude, strings.TrimPrefix(g, "!"))
		} else {
			include = append(include, g)
		}
	}
	type plan struct {
		path     string
		count    int
		replaced string
	}
	var plans []plan
	var total int
	overLimit := false
	contract.Walk(contract.WalkOption{
		Root:        root,
		MaxDepth:    64,
		MaxEntries:  100000,
		IncludeGlob: include,
		ExcludeGlob: exclude,
		SkipHidden:  true,
	}, func(abs, rel string) bool {
		data, err := os.ReadFile(abs)
		if err != nil || !utf8.Valid(data) {
			return true
		}
		count := len(re.FindAllString(string(data), -1))
		if count == 0 {
			return true
		}
		total += count
		if total > maxReplacements {
			overLimit = true
			return false
		}
		plans = append(plans, plan{path: abs, count: count, replaced: string(data)})
		return true
	})
	if overLimit {
		return contract.Fail("OUTPUT_LIMIT", fmt.Sprintf("maxReplacements %d exceeded; no files were changed", maxReplacements), false)
	}
	items := make([]json.RawMessage, 0, len(plans))
	var changed int
	for _, p := range plans {
		next := re.ReplaceAllString(p.replaced, replacement)
		if writeMode {
			tmp := p.path + ".oc-ops-tmp"
			if err := os.WriteFile(tmp, []byte(next), 0o644); err != nil {
				continue
			}
			if err := os.Rename(tmp, p.path); err != nil {
				_ = os.Remove(tmp)
				continue
			}
			changed += p.count
		}
		items = append(items, contract.ToRaw(Item{
			"path":         filepath.ToSlash(p.path),
			"replacements": p.count,
			"bytesBefore":  len(p.replaced),
			"bytesAfter":   len(next),
		}))
	}
	phase := "observe"
	if writeMode {
		phase = "verify"
	}
	return contract.Response{
		Status: "completed", Phase: phase, Changed: changed, Matched: total,
		Total: total, Complete: true, Items: items,
	}, nil
}
