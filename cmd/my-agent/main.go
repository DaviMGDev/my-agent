package main

import (
	"fmt"
	"os"

	"my-agent/internal/agent"
	"my-agent/internal/llm"
	"my-agent/internal/providers/ollama"
	"my-agent/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

// Version is set at build time via -ldflags.
// Defaults to "dev" when built without ldflags.
var Version = "dev"

func main() {
	// Support both -version and --version (Go's flag package handles -- as
	// end-of-flags, so we check os.Args directly for the double-dash form).
	for _, arg := range os.Args[1:] {
		if arg == "-version" || arg == "--version" {
			fmt.Println("my-agent version", Version)
			os.Exit(0)
		}
	}

	// --- Agent wiring ---

	llmProvider := &ollama.OllamaLLM{}
	ag := &agent.FunctionCallingAgent{LLM: llmProvider}

	registeredTools := []llm.Tool{
		&tools.GetTimeTool{},
		&tools.ReadFileTool{},
	}

	// --- TUI setup ---

	m := initialModel()
	m.agent = ag
	m.modelName = "gemma4:31b-cloud"
	m.tools = registeredTools

	p := tea.NewProgram(&m, tea.WithAltScreen())
	m.program = p

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
