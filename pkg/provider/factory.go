package provider

import "fmt"

// New creates a Provider by name.
// Supported: "anthropic", "openai".
// For openai-compatible endpoints, use NewOpenAI with WithBaseURL directly.
func New(name, apiKey, model string) (Provider, error) {
	switch name {
	case "anthropic", "claude":
		return NewAnthropic(apiKey, model), nil
	case "openai", "gpt":
		return NewOpenAI(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown provider: %q (supported: anthropic, openai)", name)
	}
}
