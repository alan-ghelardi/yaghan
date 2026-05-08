package cli

import "errors"

var ErrInterrupt = errors.New("program interrupted")

// Prompter shows interactive prompts in the terminal.
type Prompter interface {
	// Cursor returns the terminal's cursor.
	Cursor() Cursor

	// ClearBelow clears the terminal screen from the specified position downward.
	ClearBelow(position Position)

	// Confirmation displays a confirmation prompt and returns true if the user chose 'yes',
	// false otherwise. Returns an error if the prompt fails.
	Confirmation(message string) (bool, error)

	// KeyInput waits for a single key press and returns the key.
	KeyInput(label string, acceptedKeys []rune) (rune, error)

	// TextInput displays a text input and returns the user's entered text.
	TextInput(label string, opts TextInputOptions) (string, error)

	// Select displays a list of items and returns the item selected by the user.
	Select(label string, items []ListItem) (any, error)
}

// Cursor abstracts operations for querying and moving the terminal's cursor.
type Cursor interface {
	// Position returns the current cursor position in the terminal.
	Position() (Position, error)

	// Move relocates the cursor to the specified position.
	Move(position Position)
}

// Position represents a cursor's row and column in the terminal.
type Position struct {
	Row    int
	Column int
}

// TextInputOptions represent options to configure a text input.
type TextInputOptions struct {
	CharLimit    uint
	Placeholder  string
	Width        uint
	ValidateFunc func(text string) error
}

// ListItem represents a selectable item in a list.
type ListItem interface {
	// Text returns the user-facing text of this item.
	Text() string

	// Value returns the underlying value of this item.
	Value() any
}
