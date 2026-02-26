package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// filterStaleEscapes is a tea.WithFilter callback that suppresses KeyMsg
// events while the input box is disabled (during the post-startup drain
// window). This deterministically prevents late-arriving terminal escape
// sequence fragments from entering the textarea, regardless of timing.
// Ctrl+C is always allowed through so the user can exit.
func filterStaleEscapes(m tea.Model, msg tea.Msg) tea.Msg {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return msg
	}

	if km.Type == tea.KeyCtrlC {
		return msg
	}

	// The appModel keeps input disabled during the drain window. While
	// disabled, suppress all key events — they can only be escape-sequence
	// fragments since the user hasn't been prompted to type yet.
	if app, ok := m.(appModel); ok && !app.inputBox.enabled {
		return nil
	}

	return msg
}
