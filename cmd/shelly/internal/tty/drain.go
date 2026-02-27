package tty

import tea "charm.land/bubbletea/v2"

// NewStaleEscapeFilter returns a tea.WithFilter callback that suppresses all
// user-input messages while input is disabled (during the post-startup drain
// window). The isInputEnabled predicate is called on each message to check
// if input is currently active.
//
// This prevents late-arriving terminal escape sequence fragments (e.g. OSC 11
// background-color replies, cursor position reports) from entering the textarea.
// Ctrl+C is always allowed through so the user can exit.
func NewStaleEscapeFilter(isInputEnabled func(tea.Model) bool) func(tea.Model, tea.Msg) tea.Msg {
	return func(m tea.Model, msg tea.Msg) tea.Msg {
		if isInputEnabled(m) {
			return msg
		}

		// While input is disabled, suppress all input-related messages.
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
}
