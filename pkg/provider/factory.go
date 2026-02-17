package provider

import "fmt"

// Config holds provider configuration.
type Config struct {
	Name    string
	APIKey  string
	Model   string
	BaseURL string // Optional: custom endpoint for OpenAI-compatible APIs
}

// New creates a Provider by name.
// Supported: "anthropic", "openai".
// For openai-compatible endpoints with custom base URLs, set BaseURL in config.
func New(name, apiKey, model string) (Provider, error) {
	return NewFromConfig(Config{Name: name, APIKey: apiKey, Model: model})
}

// NewFromConfig creates a Provider from a full Config.
func NewFromConfig(cfg Config) (Provider, error) {
	switch cfg.Name {
	case "anthropic", "claude":
		return NewAnthropic(cfg.APIKey, cfg.Model), nil
	case "openai", "gpt":
		p := NewOpenAI(cfg.APIKey, cfg.Model)
		if cfg.BaseURL != "" {
			p = NewOpenAI(cfg.APIKey, cfg.Model, WithBaseURL(cfg.BaseURL))
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider: %q (supported: anthropic, openai)", cfg.Name)
	}
}
