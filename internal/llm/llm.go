package llm

import (
	"context"
	"fmt"
	"time"
)

// LLM abstracts a language model provider with chat completion,
// single-turn text completion, and streaming chat completion methods.
type LLM interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Complete(ctx context.Context, prompt string) (string, error)
	// StreamChat returns a ChatStream that yields chunks incrementally.
	// The caller MUST call Close() when finished, regardless of whether
	// iteration completes naturally.
	StreamChat(ctx context.Context, req *ChatRequest) (ChatStream, error)
}

// MessageRole identifies the sender of a chat message.
type MessageRole string

const (
	// RoleSystem indicates a system-level instruction message.
	RoleSystem    MessageRole = "system"
	// RoleUser indicates a message from the end user.
	RoleUser      MessageRole = "user"
	// RoleAssistant indicates a message from the AI assistant.
	RoleAssistant MessageRole = "assistant"
	// RoleTool indicates a message containing a tool call result.
	RoleTool       MessageRole = "tool"
)

// Message represents a single message in a chat conversation.
type Message struct {
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

// ChatRequest contains the parameters for a chat completion request.
type ChatRequest struct {
	Messages      []Message `json:"messages"`
	Model         string    `json:"model"`
	Temperature   float64   `json:"temperature"`
	MaxTokens     int       `json:"max_tokens"`
	StopSequences []string  `json:"stop_sequences"`
	Tools         []Tool    `json:"-"`
}

// ToolDefs returns the serializable tool definitions for all registered tools.
func (r *ChatRequest) ToolDefs() []ToolDef {
	if len(r.Tools) == 0 {
		return nil
	}
	defs := make([]ToolDef, len(r.Tools))
	for i, tool := range r.Tools {
		defs[i] = ToolDef{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Schema(),
			},
		}
	}
	return defs
}

// ChatResponse contains the result of a chat completion call.
type ChatResponse struct {
	Message      Message      `json:"message"`
	Model        string       `json:"model"`
	Usage        UsageStats   `json:"usage"`
	FinishReason FinishReason `json:"finish_reason"`
}

// UsageStats contains token counts for an LLM API call.
type UsageStats struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// FinishReason explains why the model stopped generating tokens.
type FinishReason string

const (
	// FinishReasonStop indicates the model finished naturally.
	FinishReasonStop          FinishReason = "stop"
	// FinishReasonLength indicates the response was cut off by max_tokens.
	FinishReasonLength        FinishReason = "length"
	// FinishReasonError indicates an error occurred during generation.
	FinishReasonError         FinishReason = "error"
	// FinishReasonContentFilter indicates the response was flagged by a content filter.
	FinishReasonContentFilter FinishReason = "content_filter"
)

// ChatStream is a streaming iterator over chat completion chunks.
// The caller MUST call Close() when finished, regardless of whether
// iteration completes naturally.
type ChatStream interface {
	// Next advances to the next chunk. Returns false when the stream
	// is exhausted (call Err() to check for errors).
	Next() bool
	// Current returns the most recently yielded chunk. Only valid
	// after a true return from Next().
	Current() ChatChunk
	// Err returns the first error encountered during streaming, if any.
	Err() error
	// Close releases any resources held by the stream. Safe to call
	// more than once.
	Close() error
}

// ChatChunk is one incremental delta from a streaming chat response.
type ChatChunk struct {
	Content     string          `json:"content"`
	Role        MessageRole     `json:"role"`
	ToolCalls   []ToolCallDelta `json:"tool_calls,omitempty"`
	FinishReason FinishReason   `json:"finish_reason,omitempty"`
	Usage       *UsageStats     `json:"usage,omitempty"`
}

// ToolCallDelta carries incremental tool call data for streaming responses.
type ToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// ToolCall represents a tool call made by the LLM in a non-streaming response.
type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// ToolDef is the serializable definition of a tool, used in API requests.
type ToolDef struct {
	Type     string        `json:"type"`
	Function ToolFunction  `json:"function"`
}

// ToolFunction describes a callable function for tool calling.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Tool is a callable function that an LLM can invoke.
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string
	// Description explains what the tool does.
	Description() string
	// Schema returns the JSON Schema for the tool's parameters.
	Schema() map[string]any
	// Execute runs the tool with the given arguments and returns the result.
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// MockLLM is an echo implementation of LLM that returns the user's input back.
// It is useful for unit testing code that depends on the LLM interface.
//
// ChunkDelay, if non-zero, causes MockChatStream to sleep for that duration
// before yielding each chunk, simulating real streaming latency.
type MockLLM struct {
	ChunkDelay time.Duration
}

var _ LLM = (*MockLLM)(nil)

func (m *MockLLM) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request cannot be nil")
	}
	content := ""
	if len(req.Messages) > 0 {
		content = req.Messages[len(req.Messages)-1].Content
	}

	// If tools are registered and the last message is from the user,
	// simulate a tool call response for testing the Agent loop.
	if len(req.Tools) > 0 && len(req.Messages) > 0 && req.Messages[len(req.Messages)-1].Role == RoleUser {
		tool := req.Tools[0]
		return &ChatResponse{
			Message: Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{
						ID: "mock_call_0",
						Function: struct {
							Name      string `json:"name,omitempty"`
							Arguments string `json:"arguments,omitempty"`
						}{
							Name:      tool.Name(),
							Arguments: "{}",
						},
					},
				},
			},
			Model: req.Model,
			Usage: UsageStats{
				PromptTokens:     len(content),
				CompletionTokens: 1,
				TotalTokens:      len(content) + 1,
			},
			FinishReason: FinishReasonStop,
		}, nil
	}

	return &ChatResponse{
		Message: Message{
			Role:    RoleAssistant,
			Content: content,
		},
		Model: req.Model,
		Usage: UsageStats{
			PromptTokens:     len(content),
			CompletionTokens: len(content),
			TotalTokens:      len(content) * 2,
		},
		FinishReason: FinishReasonStop,
	}, nil
}

func (m *MockLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return prompt, nil
}

// MockChatStream is an iterator over pre-built ChatChunks for testing.
type MockChatStream struct {
	chunks     []ChatChunk
	pos        int
	closed     bool
	chunkDelay time.Duration
}

var _ ChatStream = (*MockChatStream)(nil)

func (s *MockChatStream) Next() bool {
	if s.closed {
		return false
	}
	if s.pos < len(s.chunks) {
		s.pos++
		if s.chunkDelay > 0 {
			time.Sleep(s.chunkDelay)
		}
		return true
	}
	return false
}

func (s *MockChatStream) Current() ChatChunk {
	if s.pos == 0 || s.pos > len(s.chunks) {
		return ChatChunk{}
	}
	return s.chunks[s.pos-1]
}

func (s *MockChatStream) Err() error { return nil }

func (s *MockChatStream) Close() error { s.closed = true; return nil }

// StreamChat returns a ChatStream that echoes the last user message content
// as a single content chunk followed by a final done chunk.
// If tools are registered, it emits a tool-call chunk instead.
func (m *MockLLM) StreamChat(ctx context.Context, req *ChatRequest) (ChatStream, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request cannot be nil")
	}
	content := ""
	if len(req.Messages) > 0 {
		content = req.Messages[len(req.Messages)-1].Content
	}

	// If tools are registered and the last message is from the user,
	// simulate a streaming tool call response.
	if len(req.Tools) > 0 && len(req.Messages) > 0 && req.Messages[len(req.Messages)-1].Role == RoleUser {
		tool := req.Tools[0]
		chunks := []ChatChunk{
			{
				Role: RoleAssistant,
				ToolCalls: []ToolCallDelta{
					{
						Index: 0,
						ID:    "mock_call_0",
						Function: struct {
							Name      string `json:"name,omitempty"`
							Arguments string `json:"arguments,omitempty"`
						}{
							Name:      tool.Name(),
							Arguments: "{}",
						},
					},
				},
			},
			{
				FinishReason: FinishReasonStop,
				Usage: &UsageStats{
					PromptTokens:     len(content),
					CompletionTokens: 1,
					TotalTokens:      len(content) + 1,
				},
			},
		}
		return &MockChatStream{chunks: chunks, chunkDelay: m.ChunkDelay}, nil
	}

	chunks := []ChatChunk{
		{
			Content: content,
			Role:    RoleAssistant,
		},
		{
			FinishReason: FinishReasonStop,
			Usage: &UsageStats{
				PromptTokens:     len(content),
				CompletionTokens: len(content),
				TotalTokens:      len(content) * 2,
			},
		},
	}
	return &MockChatStream{chunks: chunks, chunkDelay: m.ChunkDelay}, nil
}
