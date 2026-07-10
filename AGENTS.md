# my-agent

> A Go-based LLM agent framework defining a generic chat completion interface with typed requests/responses and a mock implementation for testing.

## Build & Development

- **Run the app**: `go run .`
- **Build binary**: `go build -o my-agent .`
- **Install dependencies**: `go mod tidy`
- **Lint**: `go vet ./...`
- **Format**: `gofmt -s -w .`

## Testing

- **Run all tests**: `go test ./...`
- **Run tests verbosely**: `go test -v ./...`
- **Run tests with coverage**: `go test -cover ./...`

> No tests have been written yet. The `MockLLM` implementation in `llm.go` is designed to simplify unit testing of code that depends on the `LLM` interface.

## Code Style

- **Language**: Go 1.24
- **Formatted with**: `gofmt` (standard Go tooling)
- **Linted with**: `go vet`
- **Naming conventions**:
  - Exported types and functions: `PascalCase` (e.g., `ChatRequest`, `MockLLM`)
  - Unexported: `camelCase`
  - Constants: `PascalCase` with descriptive names (e.g., `RoleSystem`, `FinishReasonStop`)
- **JSON tags**: Used on all serializable struct fields (e.g., `json:"role"`)
- **Error handling**: idiomatic Go — functions return errors as last return value
- **Interface design**: Small, focused interfaces (`LLM` with `Chat` and `Complete` methods)

## Architecture

- **Pattern**: Interface-based design — the `LLM` interface abstracts provider-specific implementations
- **Structure**: Flat package (`package main`) — suitable for early-stage prototyping
- **Key files**:
  - `llm.go` — `LLM` interface definition, request/response types, message roles, usage stats, and `MockLLM` implementation
  - `main.go` — entry point demonstrating usage of the mock implementation

## Dependencies

- **Current state**: Zero external dependencies (stdlib only: `context`, `fmt`)
- **Design intent**: Providers can be added as new types implementing the `LLM` interface, keeping the core lightweight

## Notes for AI Agents

- This is an early-stage project with a clean, minimal surface. The `LLM` interface is the primary abstraction point — any provider (OpenAI, Anthropic, Ollama, etc.) should implement `Chat(ctx, *ChatRequest)` and `Complete(ctx, prompt)`.
- The `FinishReason` enum and `UsageStats` struct align with common LLM API patterns, making integration straightforward.
- When adding a new provider, add a new file (e.g., `openai.go`, `anthropic.go`) with a struct that implements the `LLM` interface.
