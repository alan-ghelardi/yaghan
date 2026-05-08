package prompt

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"golang.nuinfra.net/ctl/pkg/machinery"
)

var (
	forestGreen = lipgloss.Color("#228B22")
)

// bubblesPrompter is a Prompter implemented on top of the Bubble Tea API.
type bubblesPrompter struct {
}

var _ machinery.Prompter = (*bubblesPrompter)(nil)

// NewPrompter returns the default Prompter implementation.
func NewPrompter() machinery.Prompter {
	return &bubblesPrompter{}
}

// Confirmation implements machinery.Prompter.
func (b *bubblesPrompter) Confirmation(message string) (bool, error) {
	key, err := b.KeyInput(fmt.Sprintf("\u26A0\ufe0f %s (y/n)", message), []rune{'y', 'n'})
	return key == 'y', err
}

// ClearBelow implements machinery.Prompter.
func (b *bubblesPrompter) ClearBelow(position machinery.Position) {
	cursor := b.Cursor()
	cursor.Move(position)
	ansi.Execute(os.Stdout, ansi.EraseScreenBelow)
}

// Cursor implements machinery.Prompter.
func (b *bubblesPrompter) Cursor() machinery.Cursor {
	return &standardCursor{}
}

// KeyInput implements machinery.Prompter.
func (b *bubblesPrompter) KeyInput(label string, acceptedKeys []rune) (rune, error) {
	model := newKeyInputModel(label, acceptedKeys)
	program := tea.NewProgram(model)
	updatedModel, err := program.Run()
	if err != nil {
		return model.pressedKey, fmt.Errorf("error rendering key input: %w", err)
	}
	model = updatedModel.(keyInputModel)
	return model.pressedKey, model.err
}

// Select implements Prompter.
func (b *bubblesPrompter) Select(label string, items []machinery.ListItem) (selectedValue any, err error) {
	model := newListModel(label, items)
	program := tea.NewProgram(model)
	updatedModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("error rendering list: %w", err)
	}

	model = updatedModel.(listModel)
	if model.selectedItem != nil {
		selectedValue = model.selectedItem.Value()
	}
	return selectedValue, model.err
}

// TextInput implements machinery.Prompter.
func (b *bubblesPrompter) TextInput(label string, opts machinery.TextInputOptions) (string, error) {
	model := newTextInputModel(label, opts)
	program := tea.NewProgram(model)
	updatedModel, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("error rendering text input: %w", err)
	}
	model = updatedModel.(textInputModel)
	return model.textInput.Value(), model.err
}

type standardCursor struct {
}

var _ machinery.Cursor = (*standardCursor)(nil)

// Move implements machinery.Cursor.
func (s *standardCursor) Move(position machinery.Position) {
	ansi.Execute(os.Stdout, ansi.CursorPosition(position.Column, position.Row))
}

// Position implements machinery.Cursor.
func (s *standardCursor) Position() (machinery.Position, error) {
	if strings.HasSuffix(os.Getenv("INSIDE_EMACS"), ",comint") {
		return machinery.Position{Row: 1, Column: 1}, nil
	}

	position, err := getCursorPosition()
	if err != nil {
		return machinery.Position{}, fmt.Errorf("unable to read current cursor's position: %w", err)
	}
	return position, nil
}

func getCursorPosition() (machinery.Position, error) {
	fd := os.Stdin.Fd()

	// Ensure we are in raw mode to read input correctly
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return machinery.Position{}, err
	}
	defer term.Restore(fd, oldState)

	ansi.Execute(os.Stdout, ansi.RequestCursorPositionReport)

	// Read the response from the terminal
	buffer := make([]byte, 32)
	n, err := os.Stdin.Read(buffer)
	if err != nil {
		return machinery.Position{}, err
	}

	// Parse response of the form "\x1b[<row>;<col>R"
	response := string(buffer[:n])
	response = strings.TrimPrefix(response, "\x1b[")
	response = strings.TrimSuffix(response, "R")

	rowAndColumn := strings.Split(response, ";")
	if len(rowAndColumn) != 2 {
		return machinery.Position{}, fmt.Errorf("invalid response from terminal: %q", response)
	}

	row, err := strconv.Atoi(rowAndColumn[0])
	if err != nil {
		return machinery.Position{}, err
	}
	column, err := strconv.Atoi(rowAndColumn[1])
	if err != nil {
		return machinery.Position{}, err
	}

	return machinery.Position{Row: row, Column: column}, nil
}
