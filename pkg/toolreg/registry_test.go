package toolreg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rcliao/teeny-orchestrator/pkg/provider"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry(0)
	if r == nil {
		t.Fatal("nil registry")
	}
	if r.timeout.Seconds() != 30 {
		t.Fatalf("expected 30s timeout, got %v", r.timeout)
	}
}

func TestRegisterAndToToolDefs(t *testing.T) {
	r := NewRegistry(0)
	r.Register(&ToolManifest{
		Name:        "test-tool",
		Binary:      "echo",
		Description: "A test",
		Commands: map[string]CommandDef{
			"hello": {Description: "Say hello", Parameters: map[string]ParameterDef{}},
			"bye":   {Description: "Say bye", Parameters: map[string]ParameterDef{}},
		},
	})

	defs := r.ToToolDefs()
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["test-tool.hello"] || !names["test-tool.bye"] {
		t.Fatalf("missing expected tool defs: %v", names)
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "my-tool")
	os.MkdirAll(toolDir, 0755)
	os.WriteFile(filepath.Join(toolDir, "tool.json"), []byte(`{
		"name": "my-tool",
		"binary": "echo",
		"description": "test",
		"commands": {
			"run": {"description": "run it", "parameters": {}}
		}
	}`), 0644)

	r := NewRegistry(0)
	if err := r.Discover([]string{dir}); err != nil {
		t.Fatalf("discover: %v", err)
	}

	defs := r.ToToolDefs()
	if len(defs) != 1 || defs[0].Name != "my-tool.run" {
		t.Fatalf("unexpected defs: %+v", defs)
	}
}

func TestDiscoverMissingDir(t *testing.T) {
	r := NewRegistry(0)
	if err := r.Discover([]string{"/nonexistent"}); err != nil {
		t.Fatalf("should skip missing dirs, got: %v", err)
	}
}

func TestExecuteEcho(t *testing.T) {
	r := NewRegistry(0)
	r.Register(&ToolManifest{
		Name:   "test",
		Binary: "echo",
		Commands: map[string]CommandDef{
			"greet": {
				Description: "greet",
				Args:        "hello world",
				Parameters:  map[string]ParameterDef{},
			},
		},
	})

	out, err := r.Execute(context.Background(), provider.ToolCall{
		ID:        "1",
		Name:      "test.greet",
		Arguments: `{}`,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "greet hello world\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	r := NewRegistry(0)
	_, err := r.Execute(context.Background(), provider.ToolCall{Name: "nope.cmd", Arguments: `{}`})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteInvalidName(t *testing.T) {
	r := NewRegistry(0)
	_, err := r.Execute(context.Background(), provider.ToolCall{Name: "nope", Arguments: `{}`})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildJSONSchema(t *testing.T) {
	schema := buildJSONSchema(map[string]ParameterDef{
		"name": {Type: "string", Description: "Name", Required: true},
		"age":  {Type: "integer", Description: "Age", Required: false, Default: 25},
	})

	if schema["type"] != "object" {
		t.Fatalf("expected object type")
	}
	props := schema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Fatalf("expected 2 properties")
	}
	req := schema["required"].([]string)
	if len(req) != 1 || req[0] != "name" {
		t.Fatalf("unexpected required: %v", req)
	}
}
