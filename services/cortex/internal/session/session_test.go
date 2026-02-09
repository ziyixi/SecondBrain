package session

import (
	"testing"
	"time"
)

func TestManagerCreateAndGet(t *testing.T) {
	mgr := NewManager()

	s := mgr.Create("sess-1", "user-1")
	if s.ID != "sess-1" {
		t.Errorf("expected session ID 'sess-1', got %q", s.ID)
	}
	if s.UserID != "user-1" {
		t.Errorf("expected user ID 'user-1', got %q", s.UserID)
	}

	got, ok := mgr.Get("sess-1")
	if !ok {
		t.Fatal("expected to find session")
	}
	if got.ID != "sess-1" {
		t.Errorf("expected session ID 'sess-1', got %q", got.ID)
	}
}

func TestManagerGetNonExistent(t *testing.T) {
	mgr := NewManager()

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("expected not to find session")
	}
}

func TestManagerDelete(t *testing.T) {
	mgr := NewManager()
	mgr.Create("sess-1", "user-1")
	mgr.Delete("sess-1")

	_, ok := mgr.Get("sess-1")
	if ok {
		t.Error("expected session to be deleted")
	}
}

func TestSessionEpisodicMemory(t *testing.T) {
	mgr := NewManager()
	s := mgr.Create("sess-1", "user-1")

	s.AddEpisodicMemory("entry 1")
	s.AddEpisodicMemory("entry 2")

	mem := s.GetEpisodicMemory()
	if len(mem) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(mem))
	}
	if mem[0] != "entry 1" || mem[1] != "entry 2" {
		t.Errorf("unexpected memory entries: %v", mem)
	}
}

func TestSessionEpisodicMemoryOverflow(t *testing.T) {
	mgr := NewManager()
	s := mgr.Create("sess-1", "user-1")

	for i := 0; i < 60; i++ {
		s.AddEpisodicMemory("entry")
	}

	mem := s.GetEpisodicMemory()
	if len(mem) != 50 {
		t.Errorf("expected 50 entries (capped), got %d", len(mem))
	}
}

func TestSessionContext(t *testing.T) {
	mgr := NewManager()
	s := mgr.Create("sess-1", "user-1")

	s.SetContext("key1", "value1")
	s.SetContext("key2", "value2")

	ctx := s.GetContext()
	if ctx["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %q", ctx["key1"])
	}
	if ctx["key2"] != "value2" {
		t.Errorf("expected key2=value2, got %q", ctx["key2"])
	}
}

func TestManagerListSessions(t *testing.T) {
	mgr := NewManager()
	mgr.Create("a", "u1")
	mgr.Create("b", "u2")

	ids := mgr.ListSessions()
	if len(ids) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(ids))
	}
}

func TestManagerCleanupExpired(t *testing.T) {
	mgr := NewManager()
	s := mgr.Create("old", "u1")
	s.mu.Lock()
	s.LastActivityAt = time.Now().Add(-2 * time.Hour)
	s.mu.Unlock()

	mgr.Create("new", "u2")

	removed := mgr.CleanupExpired(1 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	_, ok := mgr.Get("old")
	if ok {
		t.Error("expected old session to be removed")
	}

	_, ok = mgr.Get("new")
	if !ok {
		t.Error("expected new session to still exist")
	}
}
