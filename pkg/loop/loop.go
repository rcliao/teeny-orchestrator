// Package loop implements the core agent tool loop.
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	ctxpkg "github.com/rcliao/teeny-orchestrator/pkg/context"
	"github.com/rcliao/teeny-orchestrator/pkg/provider"
	"github.com/rcliao/teeny-orchestrator/pkg/session"
	"github.com/rcliao/teeny-orchestrator/pkg/toolreg"
)

// Config for the agent loop.
type Config struct {
	MaxIterations int
	SessionKey    string
	Verbose       bool
	AutoCapture   bool   // Record calls to token-eval
	EvalBinary    string // Path to token-eval binary
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxIterations: 20,
		SessionKey:    "main",
		AutoCapture:   true,
		EvalBinary:    "token-eval",
	}
}

// AgentLoop is the core orchestrator loop.
type AgentLoop struct {
	provider   provider.Provider
	registry   *toolreg.Registry
	ctxBuilder *ctxpkg.Builder
	sessions   *session.Manager
	cfg        Config
}

// New creates an agent loop.
func New(p provider.Provider, reg *toolreg.Registry, cb *ctxpkg.Builder, sm *session.Manager, cfg Config) *AgentLoop {
	return &AgentLoop{
		provider:   p,
		registry:   reg,
		ctxBuilder: cb,
		sessions:   sm,
		cfg:        cfg,
	}
}

// Run processes a user message through the full agent loop.
// Returns the final assistant text response.
func (al *AgentLoop) Run(ctx context.Context, userMessage string) (string, error) {
	key := al.cfg.SessionKey

	// Load history and summary
	history := al.sessions.GetHistory(key)
	summary := al.sessions.GetSummary(key)

	// Build initial messages
	messages := al.ctxBuilder.BuildMessages(history, summary, userMessage)

	// Save user message to session
	al.sessions.AddMessage(key, provider.Message{Role: "user", Content: userMessage})

	// Get tool definitions
	toolDefs := al.registry.ToToolDefs()

	// Tool loop
	var finalContent string
	for i := 0; i < al.cfg.MaxIterations; i++ {
		if al.cfg.Verbose {
			log.Printf("[loop] iteration %d/%d, %d messages", i+1, al.cfg.MaxIterations, len(messages))
		}

		// Call LLM
		resp, err := al.provider.Chat(ctx, provider.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", fmt.Errorf("LLM call failed (iteration %d): %w", i+1, err)
		}

		// Auto-capture to token-eval
		if al.cfg.AutoCapture {
			al.captureEval(resp, userMessage, i+1)
		}

		if al.cfg.Verbose {
			log.Printf("[loop] response: %d chars, %d tool calls, usage: %d+%d tokens",
				len(resp.Content), len(resp.ToolCalls),
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
		}

		// No tool calls → done
		if len(resp.ToolCalls) == 0 {
			finalContent = resp.Content
			break
		}

		// Append assistant message with tool calls
		assistantMsg := provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)
		al.sessions.AddMessage(key, assistantMsg)

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			if al.cfg.Verbose {
				log.Printf("[loop] executing tool: %s(%s)", tc.Name, truncate(tc.Arguments, 100))
			}

			result, err := al.registry.Execute(ctx, tc)
			if err != nil {
				result = fmt.Sprintf("Error: %s", err)
			}

			if al.cfg.Verbose {
				log.Printf("[loop] tool result: %s", truncate(result, 200))
			}

			toolMsg := provider.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolMsg)
			al.sessions.AddMessage(key, toolMsg)
		}

		// If this was the last iteration, the next LLM call won't happen
		if i == al.cfg.MaxIterations-1 {
			finalContent = resp.Content
			if finalContent == "" {
				finalContent = "[max iterations reached]"
			}
		}
	}

	if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	// Save assistant response
	al.sessions.AddMessage(key, provider.Message{Role: "assistant", Content: finalContent})
	al.sessions.Save(key)

	return finalContent, nil
}

// captureEval records the LLM call to token-eval if available.
func (al *AgentLoop) captureEval(resp *provider.ChatResponse, intent string, iteration int) {
	binary := al.cfg.EvalBinary
	if binary == "" {
		return
	}
	// Check if binary exists
	if _, err := exec.LookPath(binary); err != nil {
		return
	}

	args := []string{
		"record",
		"--provider", al.provider.Name(),
		"--prompt-tokens", fmt.Sprintf("%d", resp.Usage.PromptTokens),
		"--completion-tokens", fmt.Sprintf("%d", resp.Usage.CompletionTokens),
		"--intent", fmt.Sprintf("orchestrator:%s:iter%d", truncate(intent, 50), iteration),
	}

	cmd := exec.Command(binary, args...)
	// Fire and forget — provide minimal JSON on stdin
	input := map[string]any{"session": al.cfg.SessionKey, "iteration": iteration}
	data, _ := json.Marshal(input)
	cmd.Stdin = strings.NewReader(string(data))
	_ = cmd.Run()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
