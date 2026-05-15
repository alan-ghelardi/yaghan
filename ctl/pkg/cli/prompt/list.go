package prompt

import (
	"fmt"
	"io"
	"strings"

	"github.com/alan-ghelardi/yaghan/ctl/pkg/cli"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	listHeight = 14
	listWidth  = 20
)

var (
	listTitleStyle        = lipgloss.NewStyle().MarginLeft(2)
	listItemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedListItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	listPaginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	listHelpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitListTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type listItemContainer struct {
	listItem cli.ListItem
}

func (i listItemContainer) FilterValue() string {
	return i.listItem.Text()
}

type listItemDelegate struct {
}

func (listItemDelegate) Height() int { return 1 }

func (listItemDelegate) Spacing() int { return 0 }

func (listItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (listItemDelegate) Render(writer io.Writer, model list.Model, index int, listItem list.Item) {
	container, ok := listItem.(listItemContainer)
	if !ok {
		return
	}

	text := fmt.Sprintf("%d. %s", index+1, container.listItem.Text())

	render := listItemStyle.Render
	if index == model.Index() {
		// This item is selected
		render = func(s ...string) string {
			return selectedListItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(writer, render(text))
}

type listModel struct {
	list         list.Model
	selectedItem cli.ListItem
	err          error
}

func newListModel(label string, items []cli.ListItem) listModel {
	list := list.New(convertToBubblesListItems(items), listItemDelegate{}, listWidth, listHeight)
	list.Title = label
	list.SetShowStatusBar(false)
	list.SetFilteringEnabled(true)
	list.Styles.Title = listTitleStyle
	list.Styles.PaginationStyle = listPaginationStyle
	list.Styles.HelpStyle = listHelpStyle

	return listModel{list: list}
}

func convertToBubblesListItems(items []cli.ListItem) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = listItemContainer{listItem: item}
	}
	return out
}

func (l listModel) Init() tea.Cmd {
	return nil
}

func (l listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.list.SetWidth(msg.Width)
		return l, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			l.err = cli.ErrInterrupt
			return l, tea.Quit

		case "enter":
			container, ok := l.list.SelectedItem().(listItemContainer)
			if ok {
				l.selectedItem = container.listItem
			}
			return l, tea.Quit
		}
	}

	var cmd tea.Cmd
	l.list, cmd = l.list.Update(msg)
	return l, cmd
}

func (l listModel) View() string {
	if l.selectedItem != nil {
		return quitListTextStyle.Render(fmt.Sprintf("Selected %s", l.selectedItem.Text()))
	}
	return "\n" + l.list.View()
}
