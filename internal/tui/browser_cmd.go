package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/cassiomarques/gh-bell/internal/browser"
)

// openBrowserCmd returns a Cmd that opens a URL in the default browser.
func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := browser.Open(url); err != nil {
			return errorMsg{err: err}
		}
		return statusMsg{text: "Opened in browser", isError: false}
	}
}
