package prompt

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

type keyInputModel struct {
	label        string
	acceptedKeys []rune
	pressedKey   rune
	err          error
}

func newKeyInputModel(label string, acceptedKeys []rune) keyInputModel {
	return keyInputModel{
		label:        label,
		acceptedKeys: acceptedKeys,
	}
}

func (k keyInputModel) Init() tea.Cmd {
	return nil
}

func (k keyInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			k.err = machinery.ErrInterrupt
			return k, tea.Quit

		default:
			key := []rune(msg.String())
			if len(key) == 1 && slices.Contains(k.acceptedKeys, key[0]) {
				k.pressedKey = key[0]
				return k, tea.Quit
			}
		}
	}
	return k, nil
}

func (k keyInputModel) View() string {
	style := lipgloss.NewStyle().
		Foreground(forestGreen).
		Bold(true).
		Underline(true).
		Padding(0, 1).
		Align(lipgloss.Center)
	output := fmt.Sprintf("%s\n(ctrl+c or esc to quit)\n", k.label)
	return style.Render(output)
}
