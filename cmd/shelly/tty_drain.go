package main

import (
	tea "charm.land/bubbletea/v2"
)

// filterStaleEscapes is a tea.WithFilter callback that suppresses all
// user-input messages while the input box is disabled (during the post-startup
// drain window). This prevents late-arriving terminal escape sequence
// fragments (e.g. OSC 11 background-color replies, cursor position reports)
// from entering the textarea, regardless of how the parser delivers them.
// Ctrl+C is always allowed through so the user can exit.
func filterStaleEscapes(m tea.Model, msg tea.Msg) tea.Msg {
	app, ok := m.(appModel)
	if !ok || app.inputBox.enabled {
		return msg
	}

	// While input is disabled, suppress all input-related messages — they can
	// only be escape-sequence fragments since the user hasn't been prompted to
	// type yet. Allow Ctrl+C through for emergency exit.
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.Key().Code == 'c' && msg.Key().Mod&tea.ModCtrl != 0 {
			return msg
		}
		return nil
	case tea.KeyReleaseMsg:
		return nil
	case tea.PasteMsg, tea.PasteStartMsg, tea.PasteEndMsg:
		return nil
	}

	return msg
}
