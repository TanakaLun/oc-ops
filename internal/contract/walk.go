package contract

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultIgnoredDirs are directory names that file operations never traverse.
var DefaultIgnoredDirs = map[string]bool{
	".git":      true,
	"node_modules": true,
	"target":    true,
	".venv":     true,
	".dsh-ops":  true,
	".oc-ops":   true,
	".gradle":   true,
	".idea":     true,
	"build":     true,
	"dist":      true,
}

// IsHidden reports whether any path component under root begins with '.'.
func IsHidden(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// IsIgnoredDir reports whether name is a directory to skip.
func IsIgnoredDir(name string) bool {
	return DefaultIgnoredDirs[name]
}

// WalkOption configures a traversal.
type WalkOption struct {
	Root        string
	MaxDepth    int
	MaxEntries  int
	MaxFileSize int64
	IncludeGlob []string // nil = all files
	ExcludeGlob []string // patterns matched by relative path
	FollowSymlinks bool
	SkipHidden  bool
}

// Walk visits every regular file under root, skipping ignored/hidden dirs and
// symlinks, stopping at MaxEntries. f receives the absolute path and relative
// path. It returns true if truncated.
func Walk(opt WalkOption, f func(abs, rel string) bool) (truncated bool) {
	if opt.MaxEntries <= 0 {
		opt.MaxEntries = 100000
	}
	if opt.MaxDepth <= 0 {
		opt.MaxDepth = 64
	}
	if opt.MaxFileSize <= 0 {
		opt.MaxFileSize = 32 << 20
	}
	count := 0
	var walk func(dir string, depth int) bool
	walk = func(dir string, depth int) bool {
		if depth > opt.MaxDepth {
			return false
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			name := e.Name()
			if opt.SkipHidden && strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			rel, _ := filepath.Rel(opt.Root, full)
			rel = filepath.ToSlash(rel)
			if e.IsDir() {
				if IsIgnoredDir(name) {
					continue
				}
				info, err := os.Lstat(full)
				if err == nil && info.Mode()&os.ModeSymlink != 0 && !opt.FollowSymlinks {
					continue
				}
				if walk(full, depth+1) {
					return true
				}
				continue
			}
			if e.Type()&os.ModeSymlink != 0 && !opt.FollowSymlinks {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Size() > opt.MaxFileSize {
				continue
			}
			if !matchGlobs(rel, opt.IncludeGlob, opt.ExcludeGlob) {
				continue
			}
			count++
			if count > opt.MaxEntries {
				return true
			}
			if !f(full, rel) {
				return true
			}
		}
		return false
	}
	walk(opt.Root, 0)
	return count > opt.MaxEntries
}

// matchGlobs implements gitignore-like include/exclude globs.
func matchGlobs(rel string, include, exclude []string) bool {
	if len(exclude) > 0 && anyGlob(exclude, rel) {
		return false
	}
	if len(include) == 0 {
		return true
	}
	return anyGlob(include, rel)
}

func anyGlob(patterns []string, rel string) bool {
	base := rel
	if idx := strings.LastIndex(rel, "/"); idx >= 0 {
		base = rel[idx+1:]
	}
	for _, p := range patterns {
		if p == "" {
			continue
		}
		neg := strings.HasPrefix(p, "!")
		pat := strings.TrimPrefix(p, "!")
		if globMatch(pat, rel) || globMatch(pat, base) {
			return !neg
		}
	}
	return false
}

// globMatch is a simplified **, *, ?, character-class glob matcher.
func globMatch(pattern, value string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	return globSegments(strings.Split(pattern, "/"), strings.Split(value, "/"))
}

func globSegments(p, v []string) bool {
	if len(p) == 0 {
		return len(v) == 0
	}
	if p[0] == "**" {
		if globSegments(p[1:], v) {
			return true
		}
		return len(v) > 0 && globSegments(p, v[1:])
	}
	if len(v) == 0 {
		return false
	}
	return segmentMatch(p[0], v[0]) && globSegments(p[1:], v[1:])
}

func segmentMatch(pattern, value string) bool {
	pi, vi, star, mark := 0, 0, -1, 0
	for vi < len(value) {
		if pi < len(pattern) {
			switch pattern[pi] {
			case '?':
				pi++
				vi++
				continue
			case '*':
				star = pi
				mark = vi
				pi++
				continue
			default:
				if pattern[pi] == value[vi] {
					pi++
					vi++
					continue
				}
			}
		}
		if star >= 0 {
			pi = star + 1
			mark++
			vi = mark
			continue
		}
		return false
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// ResolvePath resolves a model path against cwd into an absolute path.
func ResolvePath(cwd, raw string) (string, error) {
	if raw == "" {
		return cwd, nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Abs(filepath.Join(cwd, raw))
}
