package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// InlineBlock is a single-line rendering primitive with powerline-style
// glyph borders (no vertical border lines — only left/right glyphs).
// Suitable for headers, status bars, footer, session items, and other
// compact UI elements.
type InlineBlock struct {
	// Content is the single line of text to display.
	Content string

	// Border controls the glyph style.
	// Supported: "none", "square", "pointed", "slanted", "round".
	// All use powerline glyphs with no top/bottom border lines.
	Border string

	// Padding is (horizontal, vertical). Vertical padding within a
	// single-line block is minimal (0-1).
	Padding [2]int

	// Background fill color. Empty string means transparent.
	Background string

	// Foreground text color. Empty string means terminal default.
	Foreground string

	// Dimmed applies the theme's muted color.
	Dimmed bool

	// Wrap controls whether long content wraps (true) or truncates (false).
	Wrap bool

	// Weight controls sizing behavior:
	//   "none" — use padding normally
	//   "fill" — fill the entire parent width, ignores padding
	//   "wrap" — size respects content size, ignores padding
	Weight string

	// Position controls horizontal alignment:
	//   "none"   — respects padding
	//   "left"   — aligns content to the left, ignores padding
	//   "right"  — aligns content to the right, ignores padding
	//   "center" — centers content, ignores padding
	Position string
}

// borderGlyphs returns the left and right border glyphs for the given border type.
func (ib InlineBlock) borderGlyphs() (string, string) {
	switch ib.Border {
	case "square":
		return "│", "│"
	case "pointed":
		return "▐", "▌"
	case "slanted":
		return "╱", "╲"
	case "round":
		return "╭", "╮"
	default:
		return "", ""
	}
}

// Render returns an ANSI-styled single-line string fitting within the given width.
func (ib InlineBlock) Render(width int) string {
	leftGlyph, rightGlyph := ib.borderGlyphs()

	// Build the base style for content.
	style := lipgloss.NewStyle()
	if ib.Foreground != "" {
		style = style.Foreground(lipgloss.Color(ib.Foreground))
	}
	if ib.Background != "" {
		style = style.Background(lipgloss.Color(ib.Background))
	}
	if ib.Dimmed {
		style = style.Faint(true)
	}

	hPad := ib.Padding[0]
	if hPad > 0 {
		style = style.PaddingLeft(hPad).PaddingRight(hPad)
	}

	// Count fixed overhead from glyphs and the mandatory single space
	// we insert between a glyph and content.
	glyphOverhead := 0
	if leftGlyph != "" {
		glyphOverhead += lipgloss.Width(leftGlyph) + 1 // glyph + space
	}
	if rightGlyph != "" {
		glyphOverhead += 1 + lipgloss.Width(rightGlyph) // space + glyph
	}

	avail := width - glyphOverhead
	if avail < 0 {
		avail = 0
	}

	// Prepare content text based on weight / position.
	content := ib.Content

	switch ib.Weight {
	case "fill":
		// Stretch content to fill available space.
		cLen := lipgloss.Width(content)
		if cLen > avail {
			content = truncateString(content, avail)
		} else if cLen < avail {
			content = content + strings.Repeat(" ", avail-cLen)
		}

	case "wrap":
		// Natural width — no adjustment.
		// If wider than avail, truncate (wrap=false case, but we keep the line).
		if lipgloss.Width(content) > avail {
			content = truncateString(content, avail)
		}

	default: // "none"
		// Apply position if set, otherwise use padding from style.
		switch ib.Position {
		case "center":
			cLen := lipgloss.Width(content)
			if cLen < avail {
				leftPad := (avail - cLen) / 2
				content = strings.Repeat(" ", leftPad) + content
			} else if cLen > avail {
				content = truncateString(content, avail)
			}
		case "right":
			cLen := lipgloss.Width(content)
			if cLen < avail {
				content = strings.Repeat(" ", avail-cLen) + content
			} else if cLen > avail {
				content = truncateString(content, avail)
			}
		case "left":
			cLen := lipgloss.Width(content)
			if cLen < avail {
				content = content + strings.Repeat(" ", avail-cLen)
			} else if cLen > avail {
				content = truncateString(content, avail)
			}
		default:
			// Respect padding only — if content overflows avail it will be
			// truncated by the layout, but we keep the line intact.
		}
	}

	// Assemble final line.
	var b strings.Builder
	if leftGlyph != "" {
		b.WriteString(leftGlyph)
		b.WriteString(" ")
	}
	b.WriteString(style.Render(content))
	if rightGlyph != "" {
		b.WriteString(" ")
		b.WriteString(rightGlyph)
	}
	return b.String()
}

// truncateString truncates s to at most maxWidth runes.
// Works correctly for ASCII content (session names, help text).
func truncateString(s string, maxWidth int) string {
	if maxWidth < 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth])
}
