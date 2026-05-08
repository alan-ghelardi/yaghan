package print

import (
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.nuinfra.net/ctl/pkg/cli"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var headerStyle = lipgloss.NewStyle().Bold(true)

// GetItemsFunc is a function that returns the items to be displayed in a table.
type GetItemsFunc[T any] func() (items []T, hasNext bool, err error)

// Table represents a generic, paginated table of items rendered in the terminal.
// It supports dynamic re-rendering and pagination controls.
type Table[T any] struct {
	// Columns defines the structure of the table, where each Column[T] represents a column's metadata.
	Columns []Column[T]

	// GetItems is a function that retrieves the table's items, enabling dynamic content updates.
	GetItems GetItemsFunc[T]

	// DisablePagination, if set to true, hides pagination controls and disables page navigation.
	DisablePagination bool
}

// Render prints a formatted table containing the table items to the standard
// output.
func (t *Table[T]) Render(ctx *cli.Context) error {
	if len(t.Columns) == 0 {
		return errors.New("no columns")
	}

	// Get items
	items, hasNext, err := t.GetItems()
	if err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	currentPosition, err := ctx.Prompter.Cursor().Position()
	if err != nil {
		return err
	}

	// Use a tabwriter with left alignment for text and right for numbers
	tw := tabwriter.NewWriter(ctx.IOStreams.Stdout, 0, 0, 2, ' ', 0)

	// Print headers
	for _, col := range t.Columns {
		fmt.Fprintf(tw, "%s\t", headerStyle.Render(col.Header))
	}
	fmt.Fprintln(tw)

	// Print rows
	for _, item := range items {
		for _, col := range t.Columns {
			text := col.FormatFunc(col.ValueFunc(item))
			fmt.Fprintf(tw, "%s\t", text)
		}
		fmt.Fprintln(tw)
	}

	tw.Flush()

	if !t.DisablePagination && hasNext {
		key, err := ctx.Prompter.KeyInput("\U0001F53D Press 'l' to load more", []rune{'l'})
		if err != nil {
			return err
		}

		if key == 'l' {
			ctx.Prompter.ClearBelow(currentPosition)
			return t.Render(ctx)
		}
	}

	return nil
}

// Column defines how to extract and format data for a given column in the Table.
type Column[T any] struct {
	// Header is the column's title displayed in the table.
	Header string

	// ValueFunc extracts the raw value from an item of type T.
	ValueFunc func(item T) any

	// FormatFunc formats the extracted value into a displayable string.
	// If nil, a default formatter (fmt.Sprint) is used.
	FormatFunc func(value any) string
}

// NewColumn creates a Column with an optional FormatFunc.
// If no FormatFunc is provided, fmt.Sprint is used as the default.
func NewColumn[T any](header string, valueFunc func(T) any, formatFunc ...func(any) string) Column[T] {
	defaultFormat := func(v any) string { return fmt.Sprint(v) }
	if len(formatFunc) > 0 {
		defaultFormat = formatFunc[0]
	}
	return Column[T]{
		Header:     header,
		ValueFunc:  valueFunc,
		FormatFunc: defaultFormat,
	}
}

func RFC3339Formatter(value any) string {
	ts := value.(*timestamppb.Timestamp)
	return ts.AsTime().Format(time.RFC3339)
}

func DurationFormatter(value any) string {
	ts := value.(*timestamppb.Timestamp)
	duration := time.Since(ts.AsTime())

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("about %d minutes ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("about %d hours ago", int(duration.Hours()))
	case duration < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	case duration < 365*24*time.Hour:
		return fmt.Sprintf("about %d months ago", int(duration.Hours()/24/30))
	default:
		return fmt.Sprintf("about %d years ago", int(duration.Hours()/24/365))
	}
}
