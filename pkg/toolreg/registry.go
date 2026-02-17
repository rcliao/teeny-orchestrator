// Package toolreg discovers and executes tools via manifests.
package toolreg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rcliao/teeny-orchestrator/pkg/provider"
)

// CommandDef defines a single command within a tool.
type CommandDef struct {
	Description string                    `json:"description"`
	Args        string                    `json:"args"`        // Template: "--namespace {namespace}"
	Stdin       bool                      `json:"stdin"`       // Whether content goes via stdin
	StdinParam  string                    `json:"stdin_param"` // Which parameter provides stdin (default: "content")
	Parameters  map[string]ParameterDef   `json:"parameters"`
}

// ParameterDef defines a tool parameter.
type ParameterDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// ToolManifest is the tool.json format.
type ToolManifest struct {
	Name        string                `json:"name"`
	Binary      string                `json:"binary"`
	Description string                `json:"description"`
	Commands    map[string]CommandDef `json:"commands"`
}

// Registry holds discovered tools.
type Registry struct {
	tools   map[string]*ToolManifest // keyed by tool name
	timeout time.Duration
}

// NewRegistry creates an empty registry.
func NewRegistry(timeout time.Duration) *Registry {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Registry{
		tools:   make(map[string]*ToolManifest),
		timeout: timeout,
	}
}

// Discover scans directories for tool.json manifests.
func (r *Registry) Discover(dirs []string) error {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // skip missing dirs
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifestPath := filepath.Join(dir, e.Name(), "tool.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			var manifest ToolManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}
			r.tools[manifest.Name] = &manifest
		}
	}
	return nil
}

// Register adds a tool manifest directly.
func (r *Registry) Register(m *ToolManifest) {
	r.tools[m.Name] = m
}

// ToToolDefs converts all registered tools to LLM tool definitions.
// Each command becomes a separate tool: "toolname.command".
func (r *Registry) ToToolDefs() []provider.ToolDef {
	var defs []provider.ToolDef
	for _, tool := range r.tools {
		for cmdName, cmd := range tool.Commands {
			fullName := tool.Name + "." + cmdName
			defs = append(defs, provider.ToolDef{
				Name:        fullName,
				Description: fmt.Sprintf("[%s] %s", tool.Name, cmd.Description),
				Parameters:  buildJSONSchema(cmd.Parameters),
			})
		}
	}
	return defs
}

func buildJSONSchema(params map[string]ParameterDef) map[string]any {
	properties := make(map[string]any)
	var required []string

	for name, p := range params {
		prop := map[string]any{
			"type":        p.Type,
			"description": p.Description,
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		properties[name] = prop
		if p.Required {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Execute runs a tool command and returns the output.
func (r *Registry) Execute(ctx context.Context, toolCall provider.ToolCall) (string, error) {
	// Parse "toolname.command"
	parts := strings.SplitN(toolCall.Name, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid tool name: %s (expected tool.command)", toolCall.Name)
	}
	toolName, cmdName := parts[0], parts[1]

	tool, ok := r.tools[toolName]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	cmdDef, ok := tool.Commands[cmdName]
	if !ok {
		return "", fmt.Errorf("unknown command: %s.%s", toolName, cmdName)
	}

	// Parse arguments from JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
		return "", fmt.Errorf("parse tool arguments: %w", err)
	}

	// Build command line
	cmdArgs := buildCommandArgs(cmdDef, args, cmdName)

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, tool.Binary, cmdArgs...)

	// Handle stdin
	if cmdDef.Stdin {
		stdinParam := cmdDef.StdinParam
		if stdinParam == "" {
			stdinParam = "content"
		}
		if val, ok := args[stdinParam]; ok {
			cmd.Stdin = strings.NewReader(fmt.Sprintf("%v", val))
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s.%s failed: %s", toolName, cmdName, errMsg)
	}

	return stdout.String(), nil
}

func buildCommandArgs(cmdDef CommandDef, args map[string]any, cmdName string) []string {
	result := []string{cmdName}

	if cmdDef.Args != "" {
		// Template-based: replace {param} with values
		expanded := cmdDef.Args
		for key, val := range args {
			placeholder := "{" + key + "}"
			expanded = strings.ReplaceAll(expanded, placeholder, fmt.Sprintf("%v", val))
		}
		// Split and append non-empty parts
		for _, part := range strings.Fields(expanded) {
			if !strings.Contains(part, "{") { // skip unreplaced placeholders
				result = append(result, part)
			}
		}
	} else {
		// Flag-based: --key value for each arg
		stdinParam := cmdDef.StdinParam
		if stdinParam == "" {
			stdinParam = "content"
		}
		for key, val := range args {
			if cmdDef.Stdin && key == stdinParam {
				continue // stdin param handled separately
			}
			result = append(result, fmt.Sprintf("--%s", key), fmt.Sprintf("%v", val))
		}
	}

	return result
}
