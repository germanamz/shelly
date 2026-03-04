package tty

import tea "charm.land/bubbletea/v2"

// InputEnabler is implemented by tea.Model types that can report whether user
// input is currently active. Models that do not implement this interface are
// assumed to have input enabled.
type InputEnabler interface {
	InputEnabled() bool
}

// NewStaleEscapeFilter returns a tea.WithFilter callback that suppresses all
// user-input messages while input is disabled (during the post-startup drain
// window). It checks whether the model implements InputEnabler; models that
// do not are assumed to have input enabled.
//
// This prevents late-arriving terminal escape sequence fragments (e.g. OSC 11
// background-color replies, cursor position reports) from entering the textarea.
// Ctrl+C is always allowed through so the user can exit.
func NewStaleEscapeFilter() func(tea.Model, tea.Msg) tea.Msg {
	return func(m tea.Model, msg tea.Msg) tea.Msg {
		if ie, ok := m.(InputEnabler); !ok || ie.InputEnabled() {
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
