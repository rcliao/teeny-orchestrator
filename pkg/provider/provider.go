// Package provider defines the LLM provider interface and common types.
package provider

import "context"

// Message represents a conversation message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolDef defines a tool for the LLM.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema object
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ChatRequest is the input to a provider.
type ChatRequest struct {
	Model     string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
}

// ChatResponse is the output from a provider.
type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
}

// Provider is the interface all LLM backends implement.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}
