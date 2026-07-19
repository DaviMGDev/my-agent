package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// InlineBlock is a single-line rendering primitive with glyph borders
// (no vertical border lines — only left/right glyphs).
// Suitable for headers, status bars, footer, session items, and other
// compact UI elements.
type InlineBlock struct {
	// Content is the single line of text to display.
	Content string

	// Border controls the glyph style.
	// Supported: "none", "square", "pointed", "slanted", "round",
	//            "powerline" (uses / glyphs for header bridging effect).
	Border string

	// Padding is (horizontal, vertical).
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
// For "powerline", it returns the background color so the caller can apply it.
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
	case "powerline":
		return "\ue0b6", "\ue0b4"
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
		if lipgloss.Width(content) > avail {
			content = truncateString(content, avail)
		}

	default: // "none"
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
			// Respect padding only.
		}
	}

	// Build the border style — for powerline, match the background color
	// so the glyphs visually bridge into the background.
	borderStyle := lipgloss.NewStyle()
	if ib.Border == "powerline" && ib.Background != "" {
		borderStyle = borderStyle.Foreground(lipgloss.Color(ib.Background))
	}

	// Assemble final line.
	var b strings.Builder
	if leftGlyph != "" {
		b.WriteString(borderStyle.Render(leftGlyph))
		b.WriteString(" ")
	}
	b.WriteString(style.Render(content))
	if rightGlyph != "" {
		b.WriteString(" ")
		b.WriteString(borderStyle.Render(rightGlyph))
	}
	return b.String()
}

// truncateString truncates s to at most maxWidth runes.
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

// visibleRunes returns the runes of s with all ANSI escape sequences stripped.
func visibleRunes(s string) []rune {
	var out []rune
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' {
			i++
			for i < len(runes) && !(runes[i] >= 'A' && runes[i] <= 'Z') && !(runes[i] >= 'a' && runes[i] <= 'z') {
				i++
			}
			i++
			continue
		}
		out = append(out, runes[i])
		i++
	}
	return out
}

// ansiPrefix extracts the leading ANSI escape sequence(s) from s.
func ansiPrefix(s string) string {
	runes := []rune(s)
	var prefix []rune
	i := 0
	for i < len(runes) && runes[i] == '\x1b' {
		prefix = append(prefix, runes[i])
		i++
		for i < len(runes) && !(runes[i] >= 'A' && runes[i] <= 'Z') && !(runes[i] >= 'a' && runes[i] <= 'z') {
			prefix = append(prefix, runes[i])
			i++
		}
		if i < len(runes) {
			prefix = append(prefix, runes[i])
			i++
		}
	}
	return string(prefix)
}

// RenderInLineBlock is a convenience function that renders a Powerline-styled
// header InLineBlock (blue background, white foreground, bold, centered) with
// / border glyphs. This is used for viewport and sidebar headers, matching
// the go-chat TUI spec.
func RenderInLineBlock(text string, totalWidth int) string {
	inner := totalWidth - 2
	if inner < 2 {
		inner = 2
	}
	runes := []rune(text)
	if len(runes) > inner {
		text = string(runes[:inner-1]) + "…"
	}
	content := headerStyle.Width(inner).Render(text)
	borderStyle := lipgloss.NewStyle().Foreground(blue)
	return borderStyle.Render("\ue0b6") + content + borderStyle.Render("\ue0b4")
}

// RenderInLineBlockStyled renders a Powerline-styled InLineBlock with a custom
// base style and border color. Used for sidebar items where the focused session
// gets the Powerline treatment.
func RenderInLineBlockStyled(base lipgloss.Style, text string, maxWidth int, borderFg lipgloss.Color) string {
	visible := visibleRunes(text)
	if len(visible) > maxWidth-2 {
		keep := maxWidth - 5
		if keep < 1 {
			text = "…"
		} else {
			prefix := ansiPrefix(text)
			trimmed := prefix + string(visible[:keep]) + "…"
			text = trimmed
		}
	}
	styled := base.Copy().
		Padding(0, 0).
		MaxWidth(maxWidth - 2).
		Render(text)
	borderStyle := lipgloss.NewStyle().Foreground(borderFg)
	return borderStyle.Render("\ue0b6") + styled + borderStyle.Render("\ue0b4")
}
