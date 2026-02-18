package model

// Model holds provider-agnostic LLM configuration.
// The zero value is valid; zero fields mean "use provider default".
type Model struct {
	Name        string
	Temperature float64
	MaxTokens   int
}
