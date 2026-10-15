// Package repomap produces a compact repository structure map and a symbol
// index. One line per directory/file/symbol, sorted by importance, bounded and
// token-efficient — the single biggest saving when exploring an unfamiliar repo.
package repomap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/oc-ops/oc-ops/internal/contract"
)

// Item is a shorthand for a compact result item.
type Item = contract.Item

const (
	MaxDepth = 12
	MaxLines = 2000
)

// codeExts are file extensions whose contents are scanned for symbols.
var codeExts = map[string]bool{
	".go": true, ".kt": true, ".kts": true, ".java": true, ".ts": true,
	".tsx": true, ".js": true, ".jsx": true, ".rs": true, ".py": true,
	".c": true, ".cc": true, ".cpp": true, ".h": true, ".hpp": true,
	".swift": true, ".scala": true, ".cs": true, ".php": true, ".rb": true,
	".sh": true,
}

// symbolRe matches a declaration-like line in a code file.
var symbolRe = regexp.MustCompile(`(?m)^\s*(?:public\s+|private\s+|protected\s+|internal\s+|export\s+|abstract\s+|open\s+|final\s+|override\s+|static\s+|fun\s+|async\s+)?(?:class\s+|interface\s+|enum\s+|object\s+|data\s+class\s+|sealed\s+class\s+|typealias\s+|struct\s+|trait\s+|protocol\s+|type\s+|func\s+|fn\s+|def\s+|function\s+|package\s+|val\s+|var\s+|const\s+|let\s+)([A-Za-z_][A-Za-z0-9_]*)`)

func Handler(req contract.Request) (contract.Response, error) {
	if unknown := unknownKeys(req.Args, "path", "depth", "mode", "glob", "offset", "limit"); unknown != "" {
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("invalid arguments: unknown key(s) %s; this tool accepts path, depth, mode, glob, offset, limit", unknown), false)
	}
	depth := argInt(req.Args, "depth", 6)
	if depth < 1 || depth > MaxDepth {
		return contract.Fail("INVALID_ARGUMENT", "depth must be 1..12", false)
	}
	mode := argString(req.Args, "mode", "both")
	if mode != "tree" && mode != "symbols" && mode != "both" {
		return contract.Fail("INVALID_ARGUMENT", "mode must be tree, symbols or both", false)
	}
	offset := argInt(req.Args, "offset", 0)
	limit := argInt(req.Args, "limit", 500)
	if offset < 0 || limit < 1 || limit > MaxLines {
		return contract.Fail("INVALID_ARGUMENT", "offset must be >= 0 and limit 1..2000", false)
	}
	root, err := contract.ResolvePath(req.Cwd, argString(req.Args, "path", "."))
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return contract.Fail("INVALID_ARGUMENT", "map path must be an existing directory", false)
	}
	var include, exclude []string
	for _, g := range argStrings(req.Args, "glob", nil) {
		if strings.HasPrefix(g, "!") {
			exclude = append(exclude, strings.TrimPrefix(g, "!"))
		} else {
			include = append(include, g)
		}
	}

	type dirNode struct {
		path  string
		rel   string
		depth int
	}
	type fileNode struct {
		path string
		rel  string
	}

	var dirs []dirNode
	var files []fileNode
	dirs = append(dirs, dirNode{path: root, rel: ".", depth: 0})

	contract.Walk(contract.WalkOption{
		Root:        root,
		MaxDepth:    depth,
		MaxEntries:  100000,
		IncludeGlob: include,
		ExcludeGlob: exclude,
		SkipHidden:  true,
	}, func(abs, rel string) bool {
		files = append(files, fileNode{path: abs, rel: rel})
		// Record intermediate dirs for the tree.
		dir := filepath.Dir(abs)
		for dir != root && strings.HasPrefix(dir, root) {
			drel, _ := filepath.Rel(root, dir)
			drel = filepath.ToSlash(drel)
			depthOf := strings.Count(drel, "/") + 1
			dirs = append(dirs, dirNode{path: dir, rel: drel, depth: depthOf})
			dir = filepath.Dir(dir)
		}
		return true
	})

	var items []json.RawMessage
	truncated := false
	if mode == "tree" || mode == "both" {
		// Build tree lines: directories then files, grouped by depth.
		sort.Slice(dirs, func(i, j int) bool {
			if dirs[i].depth != dirs[j].depth {
				return dirs[i].depth < dirs[j].depth
			}
			return dirs[i].rel < dirs[j].rel
		})
		sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
		seen := map[string]bool{}
		emit := func(rel, kind string) bool {
			if len(items) >= limit {
				truncated = true
				return false
			}
			items = append(items, contract.ToRaw(Item{"path": rel, "kind": kind}))
			return true
		}
		for _, d := range dirs {
			if d.rel == "." {
				continue
			}
			if seen[d.rel] {
				continue
			}
			seen[d.rel] = true
			if !emit(d.rel, "dir") {
				return contract.Paged(items, offset, limit, truncated), nil
			}
		}
		for _, f := range files {
			if !emit(f.rel, "file") {
				return contract.Paged(items, offset, limit, truncated), nil
			}
		}
	}
	if mode == "symbols" || mode == "both" {
		// Symbol index: scan code files, collect declarations.
		var syms []sym
		for _, f := range files {
			if !codeExts[strings.ToLower(filepath.Ext(f.path))] {
				continue
			}
			collectSymbols(f.path, f.rel, &syms)
		}
		sort.SliceStable(syms, func(i, j int) bool { return syms[i].path < syms[j].path })
		// Deduplicate on path:line:name.
		seen := map[string]bool{}
		for _, s := range syms {
			key := fmt.Sprintf("%s:%d:%s", s.path, s.line, s.name)
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(items) >= limit {
				truncated = true
				break
			}
			items = append(items, contract.ToRaw(Item{
				"path": s.path, "line": s.line, "name": s.name, "kind": s.kind,
			}))
		}
	}
	return contract.Paged(items, offset, limit, truncated), nil
}

func collectSymbols(abs, rel string, out *[]sym) {
	f, err := os.Open(abs)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "*") {
			continue
		}
		m := symbolRe.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		name := m[1]
		kind := inferKind(line)
		*out = append(*out, sym{path: rel, line: lineNo, name: name, kind: kind})
	}
}

type sym struct {
	path string
	line int
	name string
	kind string
}

func inferKind(line string) string {
	switch {
	case strings.Contains(line, "class "), strings.Contains(line, "data class "), strings.Contains(line, "sealed class "):
		return "class"
	case strings.Contains(line, "interface "), strings.Contains(line, "trait "), strings.Contains(line, "protocol "):
		return "interface"
	case strings.Contains(line, "enum "):
		return "enum"
	case strings.Contains(line, "object "), strings.Contains(line, "struct "):
		return "object"
	case strings.Contains(line, "fun "), strings.Contains(line, "func "), strings.Contains(line, "fn "), strings.Contains(line, "def "), strings.Contains(line, "function "):
		return "func"
	case strings.Contains(line, "type "), strings.Contains(line, "typealias "):
		return "type"
	default:
		return "decl"
	}
}

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
