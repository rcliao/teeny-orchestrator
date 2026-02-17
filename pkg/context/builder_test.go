package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcliao/teeny-orchestrator/pkg/provider"
	"github.com/rcliao/teeny-orchestrator/pkg/toolreg"
)

func TestBuildMessagesStructure(t *testing.T) {
	workspace := t.TempDir()
	b := NewBuilder(workspace, DefaultConfig(), nil)

	history := []provider.Message{
		{Role: "user", Content: "prev"},
		{Role: "assistant", Content: "prev-reply"},
	}
	msgs := b.BuildMessages(history, "", "hello")

	if len(msgs) != 4 { // system + 2 history + user
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatal("first message should be system")
	}
	if msgs[3].Role != "user" || msgs[3].Content != "hello" {
		t.Fatalf("last message should be user 'hello', got %+v", msgs[3])
	}
}

func TestBuildSystemPromptIncludesSummary(t *testing.T) {
	b := NewBuilder(t.TempDir(), DefaultConfig(), nil)
	prompt := b.BuildSystemPrompt("previous conversation summary")
	if !strings.Contains(prompt, "previous conversation summary") {
		t.Fatal("summary not in system prompt")
	}
}

func TestBootstrapFileLoading(t *testing.T) {
	workspace := t.TempDir()
	os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("# Agents Config"), 0644)
	os.WriteFile(filepath.Join(workspace, "SOUL.md"), []byte("# Soul"), 0644)

	b := NewBuilder(workspace, DefaultConfig(), nil)
	prompt := b.BuildSystemPrompt("")
	if !strings.Contains(prompt, "Agents Config") {
		t.Fatal("AGENTS.md content missing")
	}
	if !strings.Contains(prompt, "Soul") {
		t.Fatal("SOUL.md content missing")
	}
}

func TestBootstrapFileTruncation(t *testing.T) {
	workspace := t.TempDir()
	bigContent := strings.Repeat("x", 30000)
	os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(bigContent), 0644)

	cfg := DefaultConfig()
	cfg.BootstrapMaxChars = 100
	b := NewBuilder(workspace, cfg, nil)
	prompt := b.BuildSystemPrompt("")
	if strings.Contains(prompt, strings.Repeat("x", 30000)) {
		t.Fatal("content not truncated")
	}
	if !strings.Contains(prompt, "[... truncated]") {
		t.Fatal("truncation marker missing")
	}
}

func TestToolSummaryInPrompt(t *testing.T) {
	reg := toolreg.NewRegistry(0)
	reg.Register(&toolreg.ToolManifest{
		Name:        "my-tool",
		Binary:      "echo",
		Description: "does stuff",
		Commands: map[string]toolreg.CommandDef{
			"do": {Description: "do the thing"},
		},
	})

	b := NewBuilder(t.TempDir(), DefaultConfig(), reg)
	prompt := b.BuildSystemPrompt("")
	if !strings.Contains(prompt, "my-tool.do") {
		t.Fatal("tool summary missing from prompt")
	}
}

func TestLearningsInjection(t *testing.T) {
	b := NewBuilder(t.TempDir(), DefaultConfig(), nil)
	b.SetLearnings("## Learnings from Previous Sessions\n\n- Use smaller prompts for simple tasks")
	prompt := b.BuildSystemPrompt("")
	if !strings.Contains(prompt, "Use smaller prompts") {
		t.Fatal("learnings not injected into system prompt")
	}
}

func TestLearningsTruncation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LearningsMaxChars = 50
	b := NewBuilder(t.TempDir(), cfg, nil)
	b.SetLearnings(strings.Repeat("learn ", 100))
	prompt := b.BuildSystemPrompt("")
	if !strings.Contains(prompt, "[... truncated]") {
		t.Fatal("learnings not truncated")
	}
}

func TestNoLearnings(t *testing.T) {
	b := NewBuilder(t.TempDir(), DefaultConfig(), nil)
	prompt := b.BuildSystemPrompt("")
	if strings.Contains(prompt, "Learnings from Previous Sessions") {
		t.Fatal("learnings section present when none set")
	}
}

func TestNoBootstrapFiles(t *testing.T) {
	b := NewBuilder(t.TempDir(), DefaultConfig(), nil)
	prompt := b.BuildSystemPrompt("")
	// Should still have identity section
	if !strings.Contains(prompt, "teeny-orchestrator") {
		t.Fatal("identity missing")
	}
}
