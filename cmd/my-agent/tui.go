package main

import (
	"context"
	"fmt"
	"strings"

	"my-agent/internal/agent"
	"my-agent/internal/llm"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Custom messages for streaming from goroutine to event loop ---

type streamChunkMsg struct {
	chunk agent.AgentChunk
}

type streamDoneMsg struct {
	err error
}

// --- Model ---

type model struct {
	// Agent wiring (set from main.go after construction)
	agent     *agent.FunctionCallingAgent
	modelName string
	tools     []llm.Tool

	// Program reference — set after tea.NewProgram so that streaming
	// goroutines can push messages back into the event loop.
	program *tea.Program

	// Sessions
	sessions  []*Session
	activeIdx int

	// UI dimensions
	width     int
	height    int
	ready     bool
	spContent int // cached sidepanel content width (inside border+padding)

	// Components
	textInput    textinput.Model
	vp           viewport.Model
	spinnerModel spinner.Model

	// Streaming state
	loading  bool
	replyBuf *strings.Builder // pointer so value-receiver Update doesn't copy a used Builder
	err      error
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Type a message…"
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 60

	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	sp.Spinner = spinner.Dot

	return model{
		sessions:     []*Session{NewSession(0, "Session 1")},
		activeIdx:    0,
		textInput:    ti,
		spinnerModel: sp,
		replyBuf:     &strings.Builder{},
	}
}

// --- Init ----------------------------------------------------------------

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, spinner.Tick)
}

// --- Update ---------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case streamChunkMsg:
		return m.handleStreamChunk(msg)

	case streamDoneMsg:
		return m.handleStreamDone(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinnerModel, cmd = m.spinnerModel.Update(msg)
		return &m, cmd

	default:
		return &m, nil
	}
}

// --- Window resize --------------------------------------------------------

func (m model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Sidepanel content width: 25% of terminal minus border+padding overhead (4).
	overhead := 4
	spTotal := m.width * 25 / 100
	if spTotal < 24 {
		spTotal = 24
	}
	m.spContent = spTotal - overhead

	// Main area content width: remaining width minus same overhead.
	mainContent := m.width - spTotal - overhead
	if mainContent < 20 {
		mainContent = 20
	}

	// Vertical layout: messages box + input box (3 content rows + 2 border rows each)
	vpHeight := m.height - 2 - 3 - 2 // total - input box - both box borders
	if vpHeight < 10 {
		vpHeight = 10
	}

	if !m.ready {
		m.vp = viewport.New(mainContent, vpHeight)
		m.ready = true
	} else {
		m.vp.Width = mainContent
		m.vp.Height = vpHeight
	}
	m.textInput.Width = mainContent

	return &m, nil
}

// --- Key handling ---------------------------------------------------------

func (m model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return &m, tea.Quit

	case tea.KeyTab:
		if !m.loading {
			m.activeIdx = (m.activeIdx + 1) % len(m.sessions)
			m.refreshViewport()
		}
		return &m, nil

	case tea.KeyShiftTab:
		if !m.loading {
			m.activeIdx = (m.activeIdx - 1 + len(m.sessions)) % len(m.sessions)
			m.refreshViewport()
		}
		return &m, nil

	case tea.KeyCtrlN:
		if !m.loading {
			id := len(m.sessions)
			sess := NewSession(id, fmt.Sprintf("Session %d", id+1))
			m.sessions = append(m.sessions, sess)
			m.activeIdx = id
			m.refreshViewport()
		}
		return &m, nil
	}

	if !m.loading && m.ready {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)

		if msg.Type == tea.KeyEnter {
			input := strings.TrimSpace(m.textInput.Value())
			if input != "" {
				m.textInput.SetValue("")
				m.startStream(input)
			}
		}
		return &m, cmd
	}

	return &m, nil
}

// --- Streaming ------------------------------------------------------------

// startStream launches the agent stream in a background goroutine.  Chunks are
// pushed into the event loop via program.Send so the model stays single-threaded.
func (m *model) startStream(input string) {
	m.loading = true
	m.replyBuf.Reset()

	sess := m.sessions[m.activeIdx]
	sess.AddMessage(llm.Message{Role: llm.RoleUser, Content: input})
	m.refreshViewport()

	go func() {
		req := &agent.AgentRequest{
			Messages: sess.Messages,
			Model:    m.modelName,
			Tools:    m.tools,
		}

		stream, err := m.agent.StreamRun(context.Background(), req)
		if err != nil {
			m.program.Send(streamDoneMsg{err: err})
			return
		}
		defer stream.Close()

		for stream.Next() {
			m.program.Send(streamChunkMsg{chunk: stream.Current()})
		}

		if err := stream.Err(); err != nil {
			m.program.Send(streamDoneMsg{err: err})
			return
		}
		m.program.Send(streamDoneMsg{})
	}()
}

func (m model) handleStreamChunk(msg streamChunkMsg) (tea.Model, tea.Cmd) {
	chunk := msg.chunk

	switch chunk.Type {
	case agent.AgentEventToken:
		m.replyBuf.WriteString(chunk.Content)

	case agent.AgentEventToolCall:
		if chunk.ToolCall != nil {
			m.replyBuf.WriteString(fmt.Sprintf(
				"\n🔧 calling %s(%s)\n",
				chunk.ToolCall.Function.Name,
				chunk.ToolCall.Function.Arguments,
			))
		}

	case agent.AgentEventToolResult:
		if chunk.ToolResult != nil {
			result := chunk.ToolResult.Result
			if len(result) > 200 {
				result = result[:200] + "..."
			}
			m.replyBuf.WriteString(fmt.Sprintf(
				"✅ %s → %s\n",
				chunk.ToolResult.Name,
				strings.TrimSpace(result),
			))
		}

	case agent.AgentEventIterationStart:
		if chunk.MaxIter > 0 {
			m.replyBuf.WriteString(fmt.Sprintf(
				"\n── iter %d/%d ──\n",
				chunk.Iteration+1,
				chunk.MaxIter,
			))
		}
	}

	sess := m.sessions[m.activeIdx]
	m.vp.SetContent(m.renderMessages(sess, m.replyBuf.String()))
	m.vp.GotoBottom()

	return &m, nil
}

func (m model) handleStreamDone(msg streamDoneMsg) (tea.Model, tea.Cmd) {
	m.loading = false

	if msg.err != nil {
		m.err = msg.err
		return &m, nil
	}

	sess := m.sessions[m.activeIdx]
	reply := m.replyBuf.String()
	m.replyBuf.Reset()

	if reply != "" {
		sess.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: reply})
	}

	m.vp.SetContent(m.renderMessages(sess, ""))
	m.vp.GotoBottom()

	return &m, nil
}

func (m model) refreshViewport() {
	if !m.ready {
		return
	}
	sess := m.sessions[m.activeIdx]
	m.vp.SetContent(m.renderMessages(sess, ""))
	m.vp.GotoBottom()
}

// --- View ----------------------------------------------------------------

func (m model) View() string {
	if !m.ready {
		return "Initializing…"
	}

	// Render sidepanel, measure its actual rendered width, then make the main
	// area fill whatever horizontal space remains.  We use lipgloss.Place to
	// force the right column to the exact remaining width, eliminating any
	// one-column gaps from lipgloss width rounding.
	left := m.renderSidepanel(m.spContent)
	leftWidth := lipgloss.Width(left)

	rightTotal := m.width - leftWidth + 2
	if rightTotal < 20 {
		rightTotal = 20
	}
	rightRaw := m.renderMain(rightTotal)
	right := lipgloss.Place(
		rightTotal,
		lipgloss.Height(rightRaw),
		lipgloss.Left,
		lipgloss.Top,
		rightRaw,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) renderSidepanel(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Sessions"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width-2))
	b.WriteString("\n\n")

	for i, sess := range m.sessions {
		if i == m.activeIdx {
			b.WriteString(activeSessionStyle.Render("▶ " + sess.Name))
		} else {
			b.WriteString(inactiveSessionStyle.Render("  " + sess.Name))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Tab next"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Ctrl+N new"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Esc quit"))

	return sidePanelStyle.Width(width).Height(m.height - 2).Render(b.String())
}

func (m model) renderMain(totalWidth int) string {
	// totalWidth is the full width for the main column including border+padding.
	// The content area inside border+padding is totalWidth-4.
	contentWidth := totalWidth - 4
	if contentWidth < 16 {
		contentWidth = 16
	}

	// Sync viewport and text input to the actual content width.
	m.vp.Width = contentWidth

	vpContent := m.vp.View()

	var inputLine string
	if m.loading {
		inputLine = m.spinnerModel.View() + " Thinking…"
	} else {
		m.textInput.Width = contentWidth
		inputLine = m.textInput.View()
	}

	msgBox := messagesStyle.Width(contentWidth).Render(vpContent)
	inputBox := inputAreaStyle.Width(contentWidth).Height(3).Render(inputLine)

	return lipgloss.JoinVertical(lipgloss.Left, msgBox, inputBox)
}

func (m model) renderMessages(sess *Session, streaming string) string {
	var b strings.Builder

	for _, msg := range sess.Messages {
		switch msg.Role {
		case llm.RoleUser:
			b.WriteString(userMsgStyle.Render("┃ " + msg.Content))
			b.WriteString("\n\n")
		case llm.RoleAssistant:
			b.WriteString(assistantMsgStyle.Render(msg.Content))
			b.WriteString("\n\n")
		case llm.RoleTool:
			b.WriteString(toolMsgStyle.Render("⚙ " + msg.Content))
			b.WriteString("\n\n")
		}
	}

	if streaming != "" {
		b.WriteString(streaming)
		b.WriteString("\n")
	}

	return b.String()
}
