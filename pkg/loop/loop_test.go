package loop

import (
	"context"
	"fmt"
	"testing"
	"time"

	ctxpkg "github.com/rcliao/teeny-orchestrator/pkg/context"
	"github.com/rcliao/teeny-orchestrator/pkg/provider"
	"github.com/rcliao/teeny-orchestrator/pkg/session"
	"github.com/rcliao/teeny-orchestrator/pkg/toolreg"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	responses []*provider.ChatResponse
	errors    []error
	calls     []provider.ChatRequest
	callIdx   int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	idx := m.callIdx
	m.calls = append(m.calls, req)
	m.callIdx++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &provider.ChatResponse{Content: "default"}, nil
}

func makeLoop(t *testing.T, mp provider.Provider, reg *toolreg.Registry) *AgentLoop {
	t.Helper()
	cb := ctxpkg.NewBuilder(t.TempDir(), ctxpkg.DefaultConfig(), reg)
	sm := session.NewManager(t.TempDir())
	cfg := DefaultConfig()
	cfg.AutoCapture = false
	return New(mp, reg, cb, sm, cfg)
}

func TestRun_SimpleResponse(t *testing.T) {
	mp := &mockProvider{
		responses: []*provider.ChatResponse{
			{Content: "Hello!", Usage: provider.Usage{PromptTokens: 10, CompletionTokens: 5}},
		},
	}
	al := makeLoop(t, mp, toolreg.NewRegistry(30*time.Second))

	result, err := al.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("got %q, want %q", result, "Hello!")
	}
	if len(mp.calls) != 1 {
		t.Errorf("expected 1 LLM call, got %d", len(mp.calls))
	}
}

func TestRun_ToolCallThenResponse(t *testing.T) {
	mp := &mockProvider{
		responses: []*provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc1", Name: "echo.run", Arguments: `{"text":"hello"}`},
				},
				Usage: provider.Usage{PromptTokens: 20, CompletionTokens: 10},
			},
			{
				Content: "Done! The echo said hello.",
				Usage:   provider.Usage{PromptTokens: 30, CompletionTokens: 15},
			},
		},
	}

	reg := toolreg.NewRegistry(30 * time.Second)
	reg.Register(&toolreg.ToolManifest{
		Name:   "echo",
		Binary: "echo",
		Commands: map[string]toolreg.CommandDef{
			"run": {
				Description: "echoes input",
				Args:        "{text}",
			},
		},
	})

	al := makeLoop(t, mp, reg)

	result, err := al.Run(context.Background(), "echo something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Done! The echo said hello." {
		t.Errorf("got %q, want %q", result, "Done! The echo said hello.")
	}
	if len(mp.calls) != 2 {
		t.Errorf("expected 2 LLM calls, got %d", len(mp.calls))
	}
}

func TestRun_LLMError(t *testing.T) {
	mp := &mockProvider{
		errors: []error{fmt.Errorf("API down")},
	}
	al := makeLoop(t, mp, toolreg.NewRegistry(30*time.Second))

	_, err := al.Run(context.Background(), "Hi")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_MaxIterations(t *testing.T) {
	mp := &mockProvider{}
	for i := 0; i < 3; i++ {
		mp.responses = append(mp.responses, &provider.ChatResponse{
			Content: fmt.Sprintf("iter %d", i),
			ToolCalls: []provider.ToolCall{
				{ID: fmt.Sprintf("tc%d", i), Name: "echo.run", Arguments: `{"text":"ok"}`},
			},
		})
	}

	reg := toolreg.NewRegistry(30 * time.Second)
	reg.Register(&toolreg.ToolManifest{
		Name:   "echo",
		Binary: "echo",
		Commands: map[string]toolreg.CommandDef{
			"run": {Description: "echo", Args: "{text}"},
		},
	})

	cb := ctxpkg.NewBuilder(t.TempDir(), ctxpkg.DefaultConfig(), reg)
	sm := session.NewManager(t.TempDir())
	cfg := DefaultConfig()
	cfg.MaxIterations = 3
	cfg.AutoCapture = false
	al := New(mp, reg, cb, sm, cfg)

	result, err := al.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result at max iterations")
	}
	if len(mp.calls) != 3 {
		t.Errorf("expected 3 LLM calls, got %d", len(mp.calls))
	}
}

func TestRun_SessionPersistence(t *testing.T) {
	mp := &mockProvider{
		responses: []*provider.ChatResponse{
			{Content: "First response"},
		},
	}
	al := makeLoop(t, mp, toolreg.NewRegistry(30*time.Second))

	_, err := al.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	history := al.sessions.GetHistory(al.cfg.SessionKey)
	if len(history) < 2 {
		t.Errorf("expected at least 2 messages in history (user + assistant), got %d", len(history))
	}
}

func TestRun_EmptyResponseFallback(t *testing.T) {
	mp := &mockProvider{
		responses: []*provider.ChatResponse{
			{Content: ""},
		},
	}
	al := makeLoop(t, mp, toolreg.NewRegistry(30*time.Second))

	result, err := al.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty content with no tool calls should still return something
	if result == "" {
		t.Error("expected non-empty fallback result")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"longer string", 5, "longe..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
