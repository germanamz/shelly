package input

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/germanamz/shelly/cmd/shelly/internal/format"
	"github.com/germanamz/shelly/cmd/shelly/internal/msgs"
	"github.com/germanamz/shelly/cmd/shelly/internal/styles"
	"github.com/germanamz/shelly/pkg/sessions"
)

const SessionPickerMaxShow = 6

// SessionPickerModel displays a popup for browsing and resuming sessions.
type SessionPickerModel struct {
	Active   bool
	sessions []sessions.SessionInfo
	filtered []sessions.SessionInfo
	query    string
	cursor   int
	maxShow  int
	Width    int
}

// NewSessionPicker creates a new SessionPickerModel.
func NewSessionPicker() SessionPickerModel {
	return SessionPickerModel{maxShow: SessionPickerMaxShow}
}

// Update processes messages for the session picker.
func (sp SessionPickerModel) Update(msg tea.Msg) (SessionPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case msgs.SessionPickerActivateMsg:
		sp.activate(msg.Sessions)
		return sp, nil
	case msgs.SessionPickerDismissMsg:
		sp.dismiss()
		return sp, nil
	case tea.KeyPressMsg:
		if !sp.Active {
			return sp, nil
		}
		return sp.handleKey(msg)
	}
	return sp, nil
}

func (sp *SessionPickerModel) activate(ss []sessions.SessionInfo) {
	sp.Active = true
	sp.sessions = ss
	sp.query = ""
	sp.cursor = 0
	sp.applyFilter()
}

func (sp *SessionPickerModel) dismiss() {
	sp.Active = false
	sp.sessions = nil
	sp.filtered = nil
	sp.query = ""
	sp.cursor = 0
}

func (sp *SessionPickerModel) handleKey(msg tea.KeyPressMsg) (SessionPickerModel, tea.Cmd) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyUp:
		if sp.cursor > 0 {
			sp.cursor--
		}
		return *sp, nil
	case tea.KeyDown:
		if sp.cursor < len(sp.filtered)-1 {
			sp.cursor++
		}
		return *sp, nil
	case tea.KeyEnter:
		if len(sp.filtered) > 0 {
			id := sp.filtered[sp.cursor].ID
			sp.dismiss()
			return *sp, func() tea.Msg { return msgs.SessionPickerSelectionMsg{ID: id} }
		}
		return *sp, nil
	case tea.KeyEsc:
		sp.dismiss()
		return *sp, nil
	case tea.KeyBackspace:
		if len(sp.query) > 0 {
			sp.query = sp.query[:len(sp.query)-1]
			sp.cursor = 0
			sp.applyFilter()
		}
		return *sp, nil
	default:
		if k.Code >= 0x20 && k.Code < 0x7f && k.Mod == 0 {
			sp.query += string(k.Code)
			sp.cursor = 0
			sp.applyFilter()
		}
		return *sp, nil
	}
}

// View renders the session picker popup.
func (sp SessionPickerModel) View() string {
	if !sp.Active {
		return ""
	}

	innerWidth := max(sp.Width-4, 30)

	var sb strings.Builder

	// Title
	sb.WriteString(styles.PickerCurStyle.Render("Sessions"))
	if sp.query != "" {
		sb.WriteString("  " + styles.PickerDimStyle.Render("filter: "+sp.query))
	}
	sb.WriteString("\n")

	if len(sp.filtered) == 0 {
		sb.WriteString(styles.PickerDimStyle.Render("  no matching sessions"))
	} else {
		show := min(len(sp.filtered), sp.maxShow)
		start := 0
		if sp.cursor >= show {
			start = sp.cursor - show + 1
		}
		end := min(start+show, len(sp.filtered))

		for i := start; i < end; i++ {
			entry := sp.filtered[i]
			preview := format.Truncate(entry.Preview, 40)
			if preview == "" {
				preview = "(empty)"
			}
			ago := relativeTime(entry.UpdatedAt)
			meta := fmt.Sprintf("%s | %s | %d msgs", entry.Agent, ago, entry.MsgCount)

			if i == sp.cursor {
				sb.WriteString(styles.PickerCurStyle.Render(preview))
				sb.WriteString("\n")
				sb.WriteString("  " + styles.PickerDimStyle.Render(meta))
			} else {
				sb.WriteString(styles.PickerDimStyle.Render(preview))
				sb.WriteString("\n")
				sb.WriteString("  " + styles.PickerDimStyle.Render(meta))
			}
			if i < end-1 {
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\n" + styles.PickerDimStyle.Render("↑↓ navigate · enter select · esc cancel"))

	border := styles.PickerBorder.Width(innerWidth)
	return border.Render(sb.String())
}

func (sp *SessionPickerModel) applyFilter() {
	if sp.query == "" {
		sp.filtered = make([]sessions.SessionInfo, len(sp.sessions))
		copy(sp.filtered, sp.sessions)
		return
	}

	q := strings.ToLower(sp.query)
	var filtered []sessions.SessionInfo
	for _, s := range sp.sessions {
		if strings.Contains(strings.ToLower(s.Preview), q) ||
			strings.Contains(strings.ToLower(s.Agent), q) {
			filtered = append(filtered, s)
		}
	}
	sp.filtered = filtered
}

// relativeTime formats a time as a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
