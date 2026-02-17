package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcliao/teeny-orchestrator/pkg/provider"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	return d
}

func TestNewManagerCreatesDir(t *testing.T) {
	d := filepath.Join(t.TempDir(), "sessions")
	m := NewManager(d)
	if m == nil {
		t.Fatal("nil manager")
	}
	if _, err := os.Stat(d); err != nil {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestAddAndGetHistory(t *testing.T) {
	m := NewManager(tempDir(t))
	m.AddMessage("s1", provider.Message{Role: "user", Content: "hello"})
	m.AddMessage("s1", provider.Message{Role: "assistant", Content: "hi"})

	h := m.GetHistory("s1")
	if len(h) != 2 {
		t.Fatalf("want 2 messages, got %d", len(h))
	}
	if h[0].Content != "hello" || h[1].Content != "hi" {
		t.Fatalf("unexpected content: %+v", h)
	}
}

func TestGetHistoryEmpty(t *testing.T) {
	m := NewManager(tempDir(t))
	h := m.GetHistory("nonexistent")
	if h != nil {
		t.Fatalf("expected nil, got %v", h)
	}
}

func TestGetHistoryIsCopy(t *testing.T) {
	m := NewManager(tempDir(t))
	m.AddMessage("s1", provider.Message{Role: "user", Content: "a"})
	h := m.GetHistory("s1")
	h[0].Content = "mutated"
	h2 := m.GetHistory("s1")
	if h2[0].Content != "a" {
		t.Fatal("GetHistory returned a reference, not a copy")
	}
}

func TestMessageCount(t *testing.T) {
	m := NewManager(tempDir(t))
	if m.MessageCount("s1") != 0 {
		t.Fatal("expected 0")
	}
	m.AddMessage("s1", provider.Message{Role: "user", Content: "x"})
	if m.MessageCount("s1") != 1 {
		t.Fatal("expected 1")
	}
}

func TestSetSummary(t *testing.T) {
	m := NewManager(tempDir(t))
	for i := 0; i < 10; i++ {
		m.AddMessage("s1", provider.Message{Role: "user", Content: "msg"})
	}
	m.SetSummary("s1", "summary text", 3)

	if m.GetSummary("s1") != "summary text" {
		t.Fatalf("summary mismatch")
	}
	if m.MessageCount("s1") != 3 {
		t.Fatalf("expected 3 messages after compaction, got %d", m.MessageCount("s1"))
	}
}

func TestSaveAndReload(t *testing.T) {
	d := tempDir(t)
	m := NewManager(d)
	m.AddMessage("test-session", provider.Message{Role: "user", Content: "persisted"})
	if err := m.Save("test-session"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload
	m2 := NewManager(d)
	h := m2.GetHistory("test-session")
	if len(h) != 1 || h[0].Content != "persisted" {
		t.Fatalf("reload failed: %+v", h)
	}
}

func TestSaveNonexistent(t *testing.T) {
	m := NewManager(tempDir(t))
	if err := m.Save("nope"); err != nil {
		t.Fatalf("save nonexistent should be nil, got %v", err)
	}
}

func TestSanitize(t *testing.T) {
	if sanitize("a:b:c") != "a_b_c" {
		t.Fatalf("sanitize failed: %s", sanitize("a:b:c"))
	}
}
