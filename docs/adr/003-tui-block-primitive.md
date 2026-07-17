# ADR 003: TUI Rendering Primitive — Single Block Model

**Status**: Proposed

**Date**: 2026-07-14

## Context

The agent currently has a REPL (`cmd/my-agent/main.go`) that renders agent events as flat `fmt.Print` lines with emoji prefixes. As the project moves toward a TUI, the question is: how many distinct widget/message types are needed to cover all renderable information the agent produces?

The agent's event stream emits these categories (see `internal/agent/agent.go`):

- User input (`llm.RoleUser`)
- Assistant text, final and streaming (`llm.RoleAssistant` + `AgentEventToken`)
- Tool call headers (`AgentEventToolCall`)
- Tool execution pending/start (`AgentEventToolStart`)
- Tool results — success and error (`AgentEventToolResult`)
- System messages (`llm.RoleSystem`)
- Iteration banners (`AgentEventIterationStart`)
- Done/final (`AgentEventDone`)
- Fatal errors, token usage, finish reason, connection status

A naive approach would create a distinct component for each. That leads to 10+ component types with duplicated rendering logic and an explosion of composition models.

The observation is that every one of these is fundamentally **a block of content with optional border, color, prefix, title, and state**. The rendering of a tool-call+result inside a colored frame is the same operation as rendering a user message with a bold prefix — only the configuration differs.

## Decision

**All TUI content is modeled as a single `Block` primitive.** Every message type — user input, assistant response, tool call, tool result, system message, error, thinking indicator — is a pre-configured `Block`. The TUI renders a vertical stream of `Block` instances in a scrollable viewport.

### The Block Struct

```go
// Block is the universal rendering primitive for the agent TUI.
// Every visual element (user input, assistant text, tool calls, errors, etc.)
// is represented as a Block with different property values.
type Block struct {
    // Content is the body text to display.
    Content string

    // ContentType controls how Content is interpreted.
    // Supported values: "markdown", "text", "json".
    ContentType string

    // Title appears in the top border line when Border != "none".
    // When Border == "none", it renders as a dimmed inline prefix above Content.
    Title string

    // Prefix is an inline character or string prepended to the first line
    // of Content. Examples: "»" (user input), "🔧" (tool call), "⚠" (error).
    Prefix string

    // --- Border ---

    // Border controls the block's visual frame.
    // Supported values: "none", "single", "double", "rounded".
    Border string

    // BorderFg is the foreground (text) color for the border and title.
    // Empty string means no color (terminal default).
    BorderFg string

    // --- Fill ---

    // Background is the background fill color for the entire block interior.
    // Empty string means transparent.
    Background string

    // Foreground is the default foreground (text) color for Content.
    // Empty string means terminal default.
    Foreground string

    // --- Semantic flags ---

    // Dimmed, when true, overrides Foreground with the theme's muted/dim color.
    // This is a semantic flag, not a color value — it ensures consistent
    // dimming across theme changes rather than baking in a specific hex value.
    Dimmed bool

    // --- Collapse ---

    // Collapsed, when true, renders only a single summary line instead of
    // the full Content. Toggled by user interaction (enter/space).
    Collapsed bool

    // Summary is the label shown when Collapsed is true.
    // Example: "System prompt (243 chars) [enter to expand]".
    Summary string

    // --- Animation ---

    // Spinner indicates an animated state within the block.
    // Supported values: "" (none), "active" (spinning), "done" (checkmark).
    // When "active", the renderer draws a spinner character at the prefix
    // position or in the border line. When "done", it draws a ✓.
    Spinner string
}
```

### Pre-configured Presets (Not Separate Types)

Every message type maps to a Block with specific property values. These are constructed via factory functions, not sub-types:

| Message | Border | Title | Prefix | ContentType | Key Property |
|---------|--------|-------|--------|-------------|-------------|
| User input | `none` | `""` | `»` | `text` | `Foreground: accent` |
| Assistant text | `none` | `""` | `""` | `markdown` | — |
| Tool call+result | `rounded` | `read_file(...)` | `🔧` | `text` | `BorderFg` transitions on state |
| System message | `none` | `""` | `ⓢ` | `text` | `Dimmed: true`, `Collapsed: true` |
| Error banner | `single` | `""` | `⚠` | `text` | `Background: red`, `Foreground: white` |
| Thinking | `none` | `"Thinking..."` | `""` | `text` | `Spinner: "active"`, `Dimmed: true` |
| Iteration separator | `none` | `"iter 2/10"` | `""` | `text` | `Dimmed: true` |

### State Transitions via Mutation

A single Block mutates through states rather than being replaced by a different component:

```
Tool call lifecycle:
  Block{Spinner:"active",  BorderFg:"muted",  Title:"read_file(path=...)"}
→ Block{Spinner:"done",    BorderFg:"green",  Title:"read_file(path=...)"}
→ Block{Spinner:"",        BorderFg:"green",  Title:"read_file(path=...)"}

Streaming text lifecycle:
  Block{Spinner:"active", Dimmed:true, Title:"Thinking..."}
→ Block{Spinner:"done", Content:"It", ...}     // first token arrives
→ Block{Spinner:"", Content:"It is 2:30 PM", ...}  // stream complete
```

### Renderer Interface

```go
// BlockRenderer converts a Block into styled terminal output at a given width.
type BlockRenderer interface {
    // Render returns the ANSI-styled string for the block.
    // The returned string must not exceed width characters per line.
    Render(block *Block, width int) string
}
```

A single implementation handles all block configurations. Border drawing, content wrapping (plain or markdown-aware), color application, and spinner animation are orthogonal concerns applied within `Render`.

## Consequences

### Positive

- **One rendering code path** — No duplicated border logic, color application, or width handling across component types.
- **Additive** — New visual variants require only a new factory function, not a new component class or renderer dispatch branch.
- **State mutations are simple assignments** — Tool lifecycle (pending→success→error) and streaming (thinking→receiving→done) are block property changes, not type swaps or component tree modifications.
- **Serializable** — A `[]Block` slice can be serialized to JSON for session persistence or debugging snapshots.
- **Testable** — `BlockRenderer.Render(block, 80)` returns a plain string. Golden-file tests compare output directly.
- **Bubble Tea compatible** — A `tea.Model` holds a `[]Block` viewport. `Update` handles key events (expand/collapse, scroll). `View` iterates blocks through `Render`.

### Negative

- **No type-safe differentiation** — The compiler cannot prevent a "user input" block from being accidentally given a border. Mitigation: factory functions enforce conventions; direct struct construction is for advanced/edge cases.
- **All-or-nothing properties** — Every block carries all fields, even when irrelevant (e.g., `Border` on a plain text block). Mitigation: zero values are no-ops; the struct is small (~10 fields).
- **No per-block interactive behavior** — Collapse/expand and scrolling are handled by the viewport model, not by individual blocks. Mitigation: this is by design — blocks are data, the viewport is the interactor.

### Risks

- **Property explosion** — Future requirements (padding, alignment, max-height, inline images) could bloat the struct. Mitigation: add properties only when needed; group related properties into sub-structs (`BlockLayout`, `BlockAnimation`) if the top-level field count exceeds ~15.
- **Markdown rendering complexity** — `ContentType: "markdown"` implies the renderer must wrap markdown-aware (don't break inside code fences, handle headings, etc.). This is the largest implementation surface. Mitigation: start with `ContentType: "text"` only; add markdown as a separate iteration after the primitives are stable.

## Rejected Alternatives

1. **One component type per message category** — Rejected because it creates ~10 component implementations with duplicated border, color, and width logic. Adding a new variant (e.g., "info panel") requires a new component.

2. **Inheritance / interface hierarchy** (`MessageWidget`, `ToolWidget`, `ErrorWidget`) — Rejected because Go lacks classical inheritance, and interface-based dispatch adds indirection without benefit when the rendering operation is the same for all variants.

3. **No primitives — raw ANSI strings in the agent loop** — Rejected because it couples rendering to the agent core, prevents theming, breaks on resize, and duplicates formatting code.

4. **Separate "container" and "leaf" primitives** — Rejected because nesting is unnecessary for a linear chat scrollback. Future needs (side panels, dialogs) may require composition, but that can be added as a viewport-level concern without changing the Block model.
