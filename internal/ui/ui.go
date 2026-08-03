package ui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	purple  = lipgloss.Color("99")
	muted   = lipgloss.Color("241")
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	Error   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	Title   = lipgloss.NewStyle().Bold(true).Foreground(purple)
	Muted   = lipgloss.NewStyle().Foreground(muted)
)

func ProjectsTable(writer io.Writer, projects []struct{ Name, Environments string }) {
	projectWidth := len("Projects")
	environmentWidth := len("Environments")
	rows := make([]table.Row, 0, len(projects))

	for _, project := range projects {
		projectWidth = max(projectWidth, lipgloss.Width(project.Name))
		environmentWidth = max(environmentWidth, lipgloss.Width(project.Environments))
		rows = append(rows, table.Row{project.Name, project.Environments})
	}

	columns := []table.Column{
		{Title: "Projects", Width: projectWidth + 2},
		{Title: "Environments", Width: environmentWidth + 2},
	}
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(muted).
		BorderBottom(true).
		Bold(false).
		Foreground(purple)
	// This table is a snapshot rather than an interactive picker, so no row
	// should appear selected.
	styles.Selected = lipgloss.NewStyle()

	model := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(max(2, len(rows)+1)),
		table.WithStyles(styles),
	)

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(muted)
	fmt.Fprintln(writer, box.Render(model.View()))
}
