package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultOllamaBaseURL is the default base URL for a local Ollama instance.
const DefaultOllamaBaseURL = "http://localhost:11434"

// OllamaLLM implements the LLM interface for Ollama's API.
//
// Zero value is usable: defaults to http://localhost:11434 and http.DefaultClient.
type OllamaLLM struct {
	// BaseURL is the Ollama server URL. Defaults to http://localhost:11434.
	BaseURL string
	// HTTPClient is the HTTP client used for API calls.
	// Defaults to http.DefaultClient.
	HTTPClient *http.Client
}

var _ LLM = (*OllamaLLM)(nil)

// --- Ollama API wire types ------------------------------------------------

type ollamaChatRequest struct {
	Model    string           `json:"model"`
	Messages []ollamaMessage  `json:"messages"`
	Stream   bool             `json:"stream"`
	Options  *ollamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	ToolCalls []ollamaToolCall  `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON object serialized as raw string
}

type ollamaOptions struct {
	Temperature float64  `json:"temperature,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

type ollamaChatResponse struct {
	Model      string         `json:"model"`
	Message    *ollamaMessage `json:"message"`
	DoneReason string         `json:"done_reason,omitempty"`
	Done       bool           `json:"done"`
	PromptEvalCount int      `json:"prompt_eval_count,omitempty"`
	EvalCount      int      `json:"eval_count,omitempty"`
}

// --- helpers --------------------------------------------------------------

func (o *OllamaLLM) url(path string) string {
	if o.BaseURL != "" {
		return o.BaseURL + path
	}
	return DefaultOllamaBaseURL + path
}

func (o *OllamaLLM) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return http.DefaultClient
}

func toOllamaRole(r MessageRole) string {
	return string(r)
}

func toMessages(msgs []Message) []ollamaMessage {
	out := make([]ollamaMessage, len(msgs))
	for i, m := range msgs {
		out[i] = ollamaMessage{Role: toOllamaRole(m.Role), Content: m.Content}
	}
	return out
}

func toOptions(req *ChatRequest) *ollamaOptions {
	if req.Temperature == 0 && req.MaxTokens == 0 && len(req.StopSequences) == 0 {
		return nil
	}
	opts := &ollamaOptions{}
	if req.Temperature != 0 {
		opts.Temperature = req.Temperature
	}
	if req.MaxTokens != 0 {
		opts.NumPredict = req.MaxTokens
	}
	if len(req.StopSequences) > 0 {
		opts.Stop = req.StopSequences
	}
	return opts
}

func toFinishReason(doneReason string) FinishReason {
	switch doneReason {
	case "stop":
		return FinishReasonStop
	case "length":
		return FinishReasonLength
	case "":
		return FinishReasonStop
	default:
		return FinishReasonError
	}
}

func toToolCallDeltas(ollamaCalls []ollamaToolCall) []ToolCallDelta {
	if len(ollamaCalls) == 0 {
		return nil
	}
	deltas := make([]ToolCallDelta, len(ollamaCalls))
	for i, tc := range ollamaCalls {
		deltas[i] = ToolCallDelta{
			Index: i,
			ID:    fmt.Sprintf("call_%d", i),
			Function: struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return deltas
}

// --- HTTP helpers ---------------------------------------------------------

func (o *OllamaLLM) doRequest(ctx context.Context, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url("/api/chat"), bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: %s (status %d)", bytes.TrimSpace(body), resp.StatusCode)
	}
	return resp, nil
}

// --- LLM interface implementation -----------------------------------------

// Chat sends a chat completion request to Ollama and returns the response.
func (o *OllamaLLM) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("ollama: chat request cannot be nil")
	}
	body := ollamaChatRequest{
		Model:    req.Model,
		Messages: toMessages(req.Messages),
		Stream:   false,
		Options:  toOptions(req),
	}

	resp, err := o.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	return o.toChatResponse(&ollamaResp, req.Model), nil
}

func (o *OllamaLLM) toChatResponse(ollamaResp *ollamaChatResponse, model string) *ChatResponse {
	cr := &ChatResponse{
		Model: model,
		FinishReason: FinishReasonStop,
	}
	if ollamaResp.Message != nil {
		cr.Message = Message{
			Role:    MessageRole(ollamaResp.Message.Role),
			Content: ollamaResp.Message.Content,
		}
	}
	if ollamaResp.DoneReason != "" {
		cr.FinishReason = toFinishReason(ollamaResp.DoneReason)
	}
	if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
		cr.Usage = UsageStats{
			PromptTokens:     ollamaResp.PromptEvalCount,
			CompletionTokens: ollamaResp.EvalCount,
			TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		}
	}
	return cr
}

// Complete sends a single-turn text completion via the chat endpoint.
func (o *OllamaLLM) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := o.Chat(ctx, &ChatRequest{
		Messages: []Message{
			{Role: RoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Message.Content, nil
}

// StreamChat returns a ChatStream that reads newline-delimited JSON from
// Ollama's streaming /api/chat endpoint.
func (o *OllamaLLM) StreamChat(ctx context.Context, req *ChatRequest) (ChatStream, error) {
	if req == nil {
		return nil, fmt.Errorf("ollama: chat request cannot be nil")
	}
	body := ollamaChatRequest{
		Model:    req.Model,
		Messages: toMessages(req.Messages),
		Stream:   true,
		Options:  toOptions(req),
	}

	resp, err := o.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	return &ollamaChatStream{
		scanner: bufio.NewScanner(resp.Body),
		body:    resp.Body,
		model:   req.Model,
	}, nil
}

// --- Streaming ------------------------------------------------------------

// ollamaChatStream implements ChatStream by reading newline-delimited JSON
// from Ollama's streaming response body.
type ollamaChatStream struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
	model   string
	current ChatChunk
	err     error
	done    bool
	closed  bool
}

func (s *ollamaChatStream) Next() bool {
	if s.closed || s.done || s.err != nil {
		return false
	}

	for s.scanner.Scan() {
		line := bytes.TrimSpace(s.scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var chunk ollamaChatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			s.err = fmt.Errorf("ollama: parse stream chunk: %w", err)
			return false
		}

		s.current = ChatChunk{
			Content: "",
			Role:    RoleAssistant,
		}
		if chunk.Message != nil {
			s.current.Content = chunk.Message.Content
			s.current.Role = MessageRole(chunk.Message.Role)
			if tcs := toToolCallDeltas(chunk.Message.ToolCalls); len(tcs) > 0 {
				s.current.ToolCalls = tcs
			}
		}

		if chunk.Done {
			s.done = true
			if chunk.DoneReason != "" {
				s.current.FinishReason = toFinishReason(chunk.DoneReason)
			} else {
				s.current.FinishReason = FinishReasonStop
			}
			if chunk.PromptEvalCount > 0 || chunk.EvalCount > 0 {
				s.current.Usage = &UsageStats{
					PromptTokens:     chunk.PromptEvalCount,
					CompletionTokens: chunk.EvalCount,
					TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
				}
			}
		}

		return true
	}

	if err := s.scanner.Err(); err != nil {
		s.err = fmt.Errorf("ollama: read stream: %w", err)
	}
	return false
}

func (s *ollamaChatStream) Current() ChatChunk {
	return s.current
}

func (s *ollamaChatStream) Err() error {
	return s.err
}

func (s *ollamaChatStream) Close() error {
	if !s.closed {
		s.closed = true
		return s.body.Close()
	}
	return nil
}


