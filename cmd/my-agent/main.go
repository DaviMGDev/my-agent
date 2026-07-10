package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"my-agent/internal/agent"
	"my-agent/internal/llm"
	"my-agent/internal/providers/ollama"
)

func main() {
	llmProvider := &ollama.OllamaLLM{}
	ag := &agent.FunctionCallingAgent{LLM: llmProvider}
	model := "ministral-3:3b-cloud"

	var messages []llm.Message
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Chat with Ollama (type /exit to quit, /clear to reset)")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		switch {
		case input == "/exit":
			fmt.Println("goodbye")
			return
		case input == "/clear":
			messages = nil
			fmt.Println("history cleared")
			continue
		case input == "":
			continue
		}

		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})

		req := &agent.AgentRequest{
			Messages: messages,
			Model:    model,
		}

		stream, err := ag.StreamRun(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			fmt.Fprintln(os.Stderr, "(is Ollama running? try: ollama serve)")
			// Remove the last user message so it can be retried
			messages = messages[:len(messages)-1]
			continue
		}

		var reply strings.Builder
		for stream.Next() {
			chunk := stream.Current()
			switch chunk.Type {
			case agent.AgentEventToken:
				fmt.Print(chunk.Content)
				reply.WriteString(chunk.Content)
			case agent.AgentEventToolCall:
				// The agent is calling a tool — no output shown to the user.
				// Tool execution and its result are handled internally by the agent.
			case agent.AgentEventToolResult:
				// Tool result received — no output shown.
				// The agent will feed this back to the LLM automatically.
			case agent.AgentEventDone:
				// Agent finished — break out of the loop.
			}
		}
		if err := stream.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "\nstream error: %v\n", err)
			messages = messages[:len(messages)-1]
			continue
		}
		stream.Close()

		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: reply.String()})
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
}
