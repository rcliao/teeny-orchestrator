// Package context builds LLM context from workspace files and session history.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rcliao/teeny-orchestrator/pkg/provider"
	"github.com/rcliao/teeny-orchestrator/pkg/toolreg"
)

// Config controls context construction limits.
type Config struct {
	BootstrapMaxChars      int // Per-file cap (default 20000)
	BootstrapTotalMaxChars int // Total cap across all files (default 24000)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BootstrapMaxChars:      20000,
		BootstrapTotalMaxChars: 24000,
	}
}

// Builder constructs LLM context.
type Builder struct {
	workspace string
	cfg       Config
	registry  *toolreg.Registry
}

// NewBuilder creates a context builder for a workspace.
func NewBuilder(workspace string, cfg Config, registry *toolreg.Registry) *Builder {
	return &Builder{
		workspace: workspace,
		cfg:       cfg,
		registry:  registry,
	}
}

// BuildMessages constructs the full message list for an LLM call.
func (b *Builder) BuildMessages(history []provider.Message, summary string, userMessage string) []provider.Message {
	systemPrompt := b.BuildSystemPrompt(summary)

	var messages []provider.Message
	messages = append(messages, provider.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, provider.Message{Role: "user", Content: userMessage})
	return messages
}

// BuildSystemPrompt assembles the system prompt from all sources.
func (b *Builder) BuildSystemPrompt(summary string) string {
	var parts []string

	// Identity
	parts = append(parts, b.buildIdentity())

	// Bootstrap files
	if bootstrap := b.loadBootstrapFiles(); bootstrap != "" {
		parts = append(parts, bootstrap)
	}

	// Tool summaries
	if toolSummary := b.buildToolSummary(); toolSummary != "" {
		parts = append(parts, toolSummary)
	}

	// Conversation summary
	if summary != "" {
		parts = append(parts, "## Previous Conversation Summary\n\n"+summary)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

func (b *Builder) buildIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	absWorkspace, _ := filepath.Abs(b.workspace)
	return fmt.Sprintf(`# teeny-orchestrator

You are an autonomous AI agent powered by teeny-claw tools.

## Current Time
%s

## Runtime
%s %s, Go %s

## Workspace
%s

## Important Rules
1. Use tools to perform actions. Do not pretend to execute commands.
2. Record important decisions and learnings to agent-memory.
3. When done with a task, mark it complete via todo-mgmt if applicable.`,
		now, runtime.GOOS, runtime.GOARCH, runtime.Version(), absWorkspace)
}

// loadBootstrapFiles reads workspace config files with budget management.
func (b *Builder) loadBootstrapFiles() string {
	files := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
		"TOOLS.md",
	}

	var parts []string
	totalChars := 0

	for _, filename := range files {
		if totalChars >= b.cfg.BootstrapTotalMaxChars {
			break
		}
		filePath := filepath.Join(b.workspace, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		content := string(data)

		// Per-file cap
		if len(content) > b.cfg.BootstrapMaxChars {
			content = content[:b.cfg.BootstrapMaxChars] + "\n\n[... truncated]"
		}

		// Total cap
		remaining := b.cfg.BootstrapTotalMaxChars - totalChars
		if len(content) > remaining {
			content = content[:remaining] + "\n\n[... truncated]"
		}

		parts = append(parts, fmt.Sprintf("## %s\n\n%s", filename, content))
		totalChars += len(content)
	}

	if len(parts) == 0 {
		return ""
	}
	return "# Workspace Context\n\n" + strings.Join(parts, "\n\n")
}

func (b *Builder) buildToolSummary() string {
	if b.registry == nil {
		return ""
	}

	defs := b.registry.ToToolDefs()
	if len(defs) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\nYou MUST use tools to perform actions.\n\n")
	for _, d := range defs {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", d.Name, d.Description))
	}
	return sb.String()
}
