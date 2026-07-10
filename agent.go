package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Agent orchestrates a tool-using conversation with an LLM.
type Agent interface {
	// Run executes a full conversation with tool calling loop.
	Run(ctx context.Context, req *AgentRequest) (*AgentResponse, error)
	// StreamRun executes the conversation and yields events for each step
	// (LLM tokens, tool call notifications, tool results).
	StreamRun(ctx context.Context, req *AgentRequest) (AgentStream, error)
}

// AgentRequest contains the parameters for an agent execution.
type AgentRequest struct {
	Messages      []Message `json:"messages"`
	Model         string    `json:"model"`
	Tools         []Tool    `json:"-"`
	Temperature   float64   `json:"temperature,omitempty"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	MaxIterations int       `json:"max_iterations,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

// AgentResponse contains the result of an agent execution.
type AgentResponse struct {
	Messages []Message   `json:"messages"`  // full conversation history
	Final    Message     `json:"final"`      // the last assistant message
	Usage    UsageStats  `json:"usage"`      // cumulative token usage
}

// AgentEventType categorises an agent stream event.
type AgentEventType string

const (
	AgentEventToken      AgentEventType = "token"       // LLM streaming token
	AgentEventToolCall   AgentEventType = "tool_call"   // LLM requested a tool
	AgentEventToolResult AgentEventType = "tool_result" // tool returned a result
	AgentEventDone       AgentEventType = "done"        // agent finished
)

// ToolResultEvent carries the result of a tool execution.
type ToolResultEvent struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Result    string         `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// AgentChunk is one event from a streaming agent execution.
type AgentChunk struct {
	Type       AgentEventType   `json:"type"`
	Content    string           `json:"content,omitempty"`
	Role       MessageRole      `json:"role,omitempty"`
	ToolCall   *ToolCallDelta   `json:"tool_call,omitempty"`
	ToolResult *ToolResultEvent `json:"tool_result,omitempty"`
	Usage      *UsageStats      `json:"usage,omitempty"`
	Done       bool             `json:"done,omitempty"`
}

// AgentStream is a streaming iterator over agent execution events.
// The caller MUST call Close() when finished.
type AgentStream interface {
	Next() bool
	Current() AgentChunk
	Err() error
	Close() error
}

// --- Concrete AgentStream -------------------------------------------------

type agentStream struct {
	ch    chan AgentChunk
	cur   AgentChunk
	err   error
	done  bool
	close sync.Once
}

func (s *agentStream) Next() bool {
	if s.done {
		return false
	}
	chunk, ok := <-s.ch
	if !ok {
		s.done = true
		return false
	}
	s.cur = chunk
	if chunk.Done {
		s.done = true
	}
	return true
}

func (s *agentStream) Current() AgentChunk { return s.cur }

func (s *agentStream) Err() error { return s.err }

func (s *agentStream) Close() error {
	s.close.Do(func() { s.done = true })
	return nil
}

// --- FunctionCallingAgent -------------------------------------------------

// FunctionCallingAgent is a concrete Agent that uses the function-calling
// pattern: call LLM → if tool calls → execute tools in parallel → feed
// results back → repeat until the LLM responds with content.
type FunctionCallingAgent struct {
	LLM        LLM
	ChunkDelay time.Duration // when >0, adds delay between stream chunks for testing
}

var _ Agent = (*FunctionCallingAgent)(nil)

// Run executes the tool-calling loop synchronously.
func (a *FunctionCallingAgent) Run(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("agent: request cannot be nil")
	}
	if a.LLM == nil {
		return nil, fmt.Errorf("agent: LLM is nil")
	}

	maxIter := req.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	messages := make([]Message, len(req.Messages))
	copy(messages, req.Messages)

	var totalUsage UsageStats

	for iter := 0; iter < maxIter; iter++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		chatReq := &ChatRequest{
			Messages:      messages,
			Model:         req.Model,
			Temperature:   req.Temperature,
			MaxTokens:     req.MaxTokens,
			StopSequences: req.StopSequences,
			Tools:         req.Tools,
		}

		resp, err := a.LLM.Chat(ctx, chatReq)
		if err != nil {
			return nil, fmt.Errorf("agent: llm chat iteration %d: %w", iter, err)
		}

		// Accumulate usage across iterations
		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		if len(resp.Message.ToolCalls) > 0 {
			// LLM wants to call tools — append assistant message with tool calls
			messages = append(messages, resp.Message)

			// Execute tools in parallel
			results := a.executeTools(ctx, resp.Message.ToolCalls, req.Tools)
			for _, tr := range results {
				messages = append(messages, Message{
					Role:    RoleTool,
					Content: tr,
				})
			}
			continue
		}

		// LLM responded with content — done
		messages = append(messages, resp.Message)
		return &AgentResponse{
			Messages: messages,
			Final:    resp.Message,
			Usage:    totalUsage,
		}, nil
	}

	return nil, fmt.Errorf("agent: max iterations (%d) exceeded without final response", maxIter)
}

// executeTools runs all tool calls in parallel and returns their results as strings.
func (a *FunctionCallingAgent) executeTools(ctx context.Context, toolCalls []ToolCall, tools []Tool) []string {
	results := make([]string, len(toolCalls))
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		i, tc := i, tc
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check context before executing
			select {
			case <-ctx.Done():
				results[i] = fmt.Sprintf(`{"error":"context cancelled"}`)
				return
			default:
			}

			var tool Tool
			for _, t := range tools {
				if t.Name() == tc.Function.Name {
					tool = t
					break
				}
			}
			if tool == nil {
				results[i] = fmt.Sprintf(`{"error":"tool %q not found"}`, tc.Function.Name)
				return
			}

			var args map[string]any
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					results[i] = fmt.Sprintf(`{"error":"failed to parse arguments: %v"}`, err)
					return
				}
			}

			result, err := tool.Execute(ctx, args)
			if err != nil {
				results[i] = fmt.Sprintf(`{"error":"%v"}`, err)
				return
			}
			results[i] = result
		}()
	}

	wg.Wait()
	return results
}

// StreamRun executes the tool-calling loop and streams events.
func (a *FunctionCallingAgent) StreamRun(ctx context.Context, req *AgentRequest) (AgentStream, error) {
	if req == nil {
		return nil, fmt.Errorf("agent: request cannot be nil")
	}
	if a.LLM == nil {
		return nil, fmt.Errorf("agent: LLM is nil")
	}

	ch := make(chan AgentChunk, 64)
	s := &agentStream{ch: ch}

	go a.streamLoop(ctx, req, ch)
	return s, nil
}

func (a *FunctionCallingAgent) streamLoop(ctx context.Context, req *AgentRequest, ch chan AgentChunk) {
	defer close(ch)

	maxIter := req.MaxIterations
	if maxIter <= 0 {
		maxIter = 10
	}

	messages := make([]Message, len(req.Messages))
	copy(messages, req.Messages)

	var totalUsage UsageStats

	for iter := 0; iter < maxIter; iter++ {
		select {
		case <-ctx.Done():
			ch <- AgentChunk{Type: AgentEventDone, Done: true}
			return
		default:
		}

		chatReq := &ChatRequest{
			Messages:      messages,
			Model:         req.Model,
			Temperature:   req.Temperature,
			MaxTokens:     req.MaxTokens,
			StopSequences: req.StopSequences,
			Tools:         req.Tools,
		}

		llmStream, err := a.LLM.StreamChat(ctx, chatReq)
		if err != nil {
			return
		}

		// Accumulate full response from streaming chunks
		var sb strings.Builder
		var toolCallDeltas []ToolCallDelta
		var finalRole MessageRole
		var finalUsage *UsageStats

		for llmStream.Next() {
			chunk := llmStream.Current()

			// Yield content tokens
			if chunk.Content != "" {
				sb.WriteString(chunk.Content)
				safeSend(ctx, ch, AgentChunk{
					Type:    AgentEventToken,
					Content: chunk.Content,
					Role:    chunk.Role,
				})
			}

			if chunk.Role != "" {
				finalRole = chunk.Role
			}
			if chunk.Usage != nil {
				finalUsage = chunk.Usage
			}
			if len(chunk.ToolCalls) > 0 {
				toolCallDeltas = append(toolCallDeltas, chunk.ToolCalls...)
			}
		}
		llmStream.Close()

		// Accumulate usage
		if finalUsage != nil {
			totalUsage.PromptTokens += finalUsage.PromptTokens
			totalUsage.CompletionTokens += finalUsage.CompletionTokens
			totalUsage.TotalTokens += finalUsage.TotalTokens
		}

		if len(toolCallDeltas) > 0 {
			// Build assistant message with tool calls
			assistantMsg := Message{
				Role:    finalRole,
				Content: sb.String(),
			}
			if finalRole == "" {
				assistantMsg.Role = RoleAssistant
			}

			// Convert deltas to ToolCall and yield events
			toolCalls := make([]ToolCall, len(toolCallDeltas))
			for i, d := range toolCallDeltas {
				toolCalls[i] = ToolCall{
					ID: d.ID,
					Function: struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{
						Name:      d.Function.Name,
						Arguments: d.Function.Arguments,
					},
				}

				// Yield tool call event
				delta := d // copy
				safeSend(ctx, ch, AgentChunk{
					Type:     AgentEventToolCall,
					ToolCall: &delta,
				})
			}

			assistantMsg.ToolCalls = toolCalls
			messages = append(messages, assistantMsg)

			// Execute tools in parallel
			results := a.executeTools(ctx, toolCalls, req.Tools)

			// Yield tool result events and append to history
			for i, tc := range toolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

				tre := &ToolResultEvent{
					Name:      tc.Function.Name,
					Arguments: args,
					Result:    results[i],
				}

				safeSend(ctx, ch, AgentChunk{
					Type:       AgentEventToolResult,
					ToolResult: tre,
				})

				messages = append(messages, Message{
					Role:    RoleTool,
					Content: results[i],
				})
			}

			// Loop back to call LLM again with tool results
			continue
		}

		// LLM responded with content — done
		assistantMsg := Message{
			Role:    finalRole,
			Content: sb.String(),
		}
		if finalRole == "" {
			assistantMsg.Role = RoleAssistant
		}
		messages = append(messages, assistantMsg)

		totalUsageCopy := totalUsage
		safeSend(ctx, ch, AgentChunk{
			Type:  AgentEventDone,
			Done:  true,
			Usage: &totalUsageCopy,
		})
		return
	}

	// Max iterations exceeded
	safeSend(ctx, ch, AgentChunk{
		Type: AgentEventDone,
		Done: true,
	})
}

// safeSend sends a chunk to the channel or aborts if context is cancelled.
func safeSend(ctx context.Context, ch chan AgentChunk, chunk AgentChunk) {
	select {
	case ch <- chunk:
	case <-ctx.Done():
	}
}

// --- Agent helpers --------------------------------------------------------

// compile-time interface check
var _ AgentStream = (*agentStream)(nil)

// --- MockAgent ------------------------------------------------------------

// MockAgent is a deterministic Agent implementation for testing.
// It returns pre-configured responses in sequence.
type MockAgent struct {
	Responses []*AgentResponse
	Err       error
	Index     int
	mu        sync.Mutex
}

var _ Agent = (*MockAgent)(nil)

func (m *MockAgent) Run(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("agent: request cannot be nil")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Err != nil {
		return nil, m.Err
	}
	if m.Index >= len(m.Responses) {
		return nil, fmt.Errorf("mock agent: no more responses (index %d)", m.Index)
	}
	resp := m.Responses[m.Index]
	m.Index++
	return resp, nil
}

func (m *MockAgent) StreamRun(ctx context.Context, req *AgentRequest) (AgentStream, error) {
	// For testing, StreamRun delegates to Run and wraps the result as a stream.
	// This is intentionally simple — tests that need fine-grained streaming
	// control should construct a custom AgentStream.
	resp, err := m.Run(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan AgentChunk, 4)
	s := &agentStream{ch: ch}

	go func() {
		defer close(ch)
		if resp.Final.Content != "" {
			ch <- AgentChunk{
				Type:    AgentEventToken,
				Content: resp.Final.Content,
				Role:    resp.Final.Role,
			}
		}
		usage := resp.Usage
		ch <- AgentChunk{
			Type:  AgentEventDone,
			Done:  true,
			Usage: &usage,
		}
	}()

	return s, nil
}

// --- MockTool -------------------------------------------------------------

// MockTool is a deterministic Tool implementation for testing.
type MockTool struct {
	NameValue        string
	DescriptionValue string
	SchemaValue      map[string]any
	ExecuteFn        func(ctx context.Context, args map[string]any) (string, error)
}

var _ Tool = (*MockTool)(nil)

func (m *MockTool) Name() string                     { return m.NameValue }
func (m *MockTool) Description() string               { return m.DescriptionValue }
func (m *MockTool) Schema() map[string]any            { return m.SchemaValue }
func (m *MockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if m.ExecuteFn == nil {
		return "mock result", nil
	}
	return m.ExecuteFn(ctx, args)
}
