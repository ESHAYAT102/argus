package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	purple  = lipgloss.Color("99")
	muted   = lipgloss.Color("241")
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	Title   = lipgloss.NewStyle().Bold(true).Foreground(purple)
	Muted   = lipgloss.NewStyle().Foreground(muted)
)

func ProjectsTable(writer io.Writer, projects []struct{ Name, Environments string }) {
	var body strings.Builder
	body.WriteString(Title.Render("Projects      Environments"))
	body.WriteString("\n")
	body.WriteString(Muted.Render(strings.Repeat("─", 52)))
	body.WriteString("\n")
	for _, project := range projects {
		body.WriteString(fmt.Sprintf("%-14s%s\n", project.Name, project.Environments))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Width(58)
	fmt.Fprintln(writer, box.Render(body.String()))
}
