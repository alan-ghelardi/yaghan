package prompt

import (
	"fmt"

	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	defaultTextInputCharLimit = 256
	defaultTextInputWidth     = 20
)

type textInputModel struct {
	textInput textinput.Model
	label     string
	err       error
}

func newTextInputModel(label string, opts cli.TextInputOptions) textInputModel {
	if opts.CharLimit == 0 {
		opts.CharLimit = defaultTextInputCharLimit
	}
	if opts.Width == 0 {
		opts.Width = defaultTextInputWidth
	}

	textInput := textinput.New()
	textInput.Placeholder = opts.Placeholder
	textInput.CharLimit = int(opts.CharLimit)
	textInput.Width = int(opts.Width)
	textInput.Validate = opts.ValidateFunc
	textInput.Focus()

	return textInputModel{
		textInput: textInput,
		label:     label,
	}
}

func (t textInputModel) Init() tea.Cmd {
	return nil
}

func (t textInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyCtrlC, tea.KeyEsc:
			t.err = cli.ErrInterrupt
			return t, tea.Quit
		}

	case error:
		t.err = msg
		return t, nil
	}

	t.textInput, cmd = t.textInput.Update(msg)
	return t, cmd
}

func (t textInputModel) View() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		t.label,
		t.textInput.View(),
		"(esc to quit)",
	) + "\n"
}
