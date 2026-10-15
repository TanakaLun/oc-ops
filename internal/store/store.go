// Package store provides a shared disk-backed JSON store with atomic
// save/load/delete/list. It is used by the PTC result references and batch
// audit records. Records survive across separate CLI invocations so a plan can
// be resumed later.
package store

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Root returns the oc-ops cache root.
func Root() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "oc-ops")
}

// Store names a record collection (result, batch).
type Store struct {
	dir string
}

// Open returns a store for the named collection.
func Open(collection string) (*Store, error) {
	dir := filepath.Join(Root(), collection)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// ValidateID rejects ids that could escape the store directory.
func ValidateID(id string) error {
	if id == "" || len(id) > 128 || id == "." || id == ".." {
		return fmt.Errorf("invalid id: must be 1..128 chars")
	}
	if id != filepath.Base(id) || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("invalid id %q", id)
	}
	return nil
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".json") }

// Save writes one record atomically (tmp + rename).
func (s *Store) Save(id string, v any) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := s.path(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(id))
}

// Load reads one record into v. Returns os.ErrNotExist when absent.
func (s *Store) Load(id string, v any) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	b, err := os.ReadFile(s.path(id))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Delete removes one record.
func (s *Store) Delete(id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// List returns the ids of all stored records.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			ids = append(ids, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Now is the epoch-seconds clock used for TTL checks.
func Now() int64 { return time.Now().Unix() }

// NewID returns a collision-safe record id with the given prefix (e.g. "r").
// It mixes nanosecond time with a per-process counter and a random suffix so
// separate CLI invocations (separate processes) never collide.
func NewID(prefix string) string {
	c := seqCounter.Add(1)
	r := randHex(4)
	return fmt.Sprintf("%s-%d-%d-%s", prefix, time.Now().UnixNano(), c, r)
}

func randHex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = digits[int(b[i])%16]
	}
	return string(b)
}

var seqCounter = &atomicCounter{}

type atomicCounter struct{ n int64 }

func (c *atomicCounter) Add(d int64) int64 { c.n += d; return c.n }
