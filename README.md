# my-agent

A Go-based LLM agent framework with a generic chat completion interface and built-in mock implementation.

## Overview

`my-agent` defines a minimal `LLM` interface that abstracts provider-specific LLM interactions behind two methods:

- **`Chat(ctx, *ChatRequest)`** — Conversational chat with message history, model selection, and generation parameters.
- **`Complete(ctx, prompt)`** — Single-turn text completion.

The project ships with a `MockLLM` implementation that returns a canned response, making it easy to write unit tests and prototype agent logic without an API key.

## Types

| Type | Description |
|------|-------------|
| `Message` | A single chat message with `role` (system/user/assistant) and `content` |
| `ChatRequest` | Input to `Chat()`: messages, model, temperature, max tokens, stop sequences |
| `ChatResponse` | Output from `Chat()`: response message, model name, token usage, finish reason |
| `UsageStats` | Token counts for prompt, completion, and total |
| `FinishReason` | Why generation stopped (`stop`, `length`, `error`, `content_filter`) |

## Getting Started

```go
package main

import (
	"context"
	"fmt"
)

func main() {
	mock := &MockLLM{}
	req := &ChatRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "Hello, how are you?"},
		},
		Model:       "mock-model",
		Temperature: 0.7,
		MaxTokens:   100,
	}

	resp, err := mock.Chat(context.Background(), req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Response:", resp.Message.Content)
}
```

```bash
go run .
# Output: Response: This is a mock response.
```

## Extending

Add a new provider by creating a file (e.g., `openai.go`) with a struct that implements the `LLM` interface:

```go
type OpenAILLM struct {
	apiKey string
}

func (o *OpenAILLM) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Real API call
}

func (o *OpenAILLM) Complete(ctx context.Context, prompt string) (string, error) {
	// Real completion call
}
```

## License

MIT — see [LICENSE](./LICENSE).
