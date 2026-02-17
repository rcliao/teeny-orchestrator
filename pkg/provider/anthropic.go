package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Anthropic implements Provider for Claude models.
type Anthropic struct {
	apiKey string
	model  string
}

// NewAnthropic creates an Anthropic provider.
// apiKey defaults to ANTHROPIC_API_KEY env var if empty.
// model defaults to claude-sonnet-4-20250514 if empty.
func NewAnthropic(apiKey, model string) *Anthropic {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &Anthropic{apiKey: apiKey, model: model}
}

func (a *Anthropic) Name() string { return "anthropic" }

// Anthropic API types

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	Type string `json:"type"`
}

func (a *Anthropic) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if a.apiKey == "" {
		return nil, fmt.Errorf("anthropic: ANTHROPIC_API_KEY not set")
	}

	model := req.Model
	if model == "" {
		model = a.model
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Convert messages: extract system, convert rest to Anthropic format
	var system string
	var msgs []anthropicMessage

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			msgs = append(msgs, anthropicMessage{Role: "user", Content: m.Content})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Assistant message with tool calls → content blocks
				var blocks []contentBlock
				if m.Content != "" {
					blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					var input any
					_ = json.Unmarshal([]byte(tc.Arguments), &input)
					blocks = append(blocks, contentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					})
				}
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
			} else {
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: m.Content})
			}
		case "tool":
			// Tool result → user message with tool_result content block
			msgs = append(msgs, anthropicMessage{
				Role: "user",
				Content: []contentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
		}
	}

	// Convert tools
	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	apiReq := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
		Tools:     tools,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	if apiResp.Type == "error" && apiResp.Error != nil {
		return nil, fmt.Errorf("anthropic: API error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	// Parse response content blocks
	result := &ChatResponse{
		Usage: Usage{
			PromptTokens:     apiResp.Usage.InputTokens,
			CompletionTokens: apiResp.Usage.OutputTokens,
		},
	}

	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}

	return result, nil
}
