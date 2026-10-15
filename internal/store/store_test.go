package store

import (
	"os"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	_ = os.RemoveAll(Root())
	s, err := Open("test-rt")
	if err != nil {
		t.Fatal(err)
	}
	type rec struct{ N int }
	if err := s.Save("rec-1", rec{N: 42}); err != nil {
		t.Fatal(err)
	}
	var out rec
	if err := s.Load("rec-1", &out); err != nil || out.N != 42 {
		t.Fatalf("round trip failed: %+v %v", out, err)
	}
	ids, err := s.List()
	if err != nil || len(ids) != 1 || ids[0] != "rec-1" {
		t.Fatalf("list failed: %v %v", ids, err)
	}
	if err := s.Delete("rec-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateID(t *testing.T) {
	for _, id := range []string{"", "a/b", "..", strings.Repeat("x", 200)} {
		if ValidateID(id) == nil {
			t.Fatalf("id %q should be rejected", id)
		}
	}
	if ValidateID("r-abc-1") != nil {
		t.Fatal("valid id rejected")
	}
}

func TestNewIDUnique(t *testing.T) {
	set := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID("r")
		if set[id] {
			t.Fatalf("collision: %s", id)
		}
		set[id] = true
	}
}
