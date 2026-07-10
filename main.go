package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func main() {
	llm := &OllamaLLM{}
	model := "ministral-3:3b-cloud"

	var messages []Message
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

		messages = append(messages, Message{Role: RoleUser, Content: input})

		req := &ChatRequest{
			Messages: messages,
			Model:    model,
		}

		stream, err := llm.StreamChat(context.Background(), req)
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
			fmt.Print(chunk.Content)
			reply.WriteString(chunk.Content)
		}
		if err := stream.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "\nstream error: %v\n", err)
			messages = messages[:len(messages)-1]
			continue
		}
		stream.Close()

		messages = append(messages, Message{Role: RoleAssistant, Content: reply.String()})
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
}
