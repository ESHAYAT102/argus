package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	charmlog "github.com/charmbracelet/log"
)

func ActivityLog(writer io.Writer, activity []api.Activity) {
	if len(activity) == 0 {
		fmt.Fprintln(writer, "Nothing has happened here yet.")
		return
	}

	logger := charmlog.NewWithOptions(writer, charmlog.Options{
		Level:           charmlog.DebugLevel,
		ReportTimestamp: true,
		TimeFormat:      "Jan 02 15:04",
	})
	styles := charmlog.DefaultStyles()
	styles.Levels[charmlog.DebugLevel] = styles.Levels[charmlog.DebugLevel].SetString("PULL")
	styles.Levels[charmlog.InfoLevel] = styles.Levels[charmlog.InfoLevel].SetString("DONE")
	styles.Levels[charmlog.WarnLevel] = styles.Levels[charmlog.WarnLevel].SetString("EDIT")
	styles.Levels[charmlog.ErrorLevel] = styles.Levels[charmlog.ErrorLevel].SetString("GONE")
	styles.Keys["actor"] = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	styles.Values["actor"] = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	styles.Keys["env"] = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	styles.Values["env"] = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	styles.Keys["variable"] = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	styles.Values["variable"] = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	logger.SetStyles(styles)

	for _, event := range activity {
		createdAt := event.CreatedAt.Local()
		logger.SetTimeFunction(func(time.Time) time.Time { return createdAt })
		fields := []any{"actor", "@" + event.Actor}
		if event.Environment != "" {
			fields = append(fields, "env", event.Environment)
		}
		if event.Variable != "" {
			fields = append(fields, "variable", event.Variable)
		}

		level, message := activityPresentation(event.Action)
		logger.Log(level, message, fields...)
	}
}

func activityPresentation(action string) (charmlog.Level, string) {
	switch action {
	case "project.initialized":
		return charmlog.InfoLevel, "Project initialized ✨"
	case "environment.created":
		return charmlog.InfoLevel, "Environment created 🌱"
	case "environment.pushed":
		return charmlog.InfoLevel, "Environment pushed ↑"
	case "environment.fetched":
		return charmlog.DebugLevel, "Environment fetched ↓"
	case "environment.pulled":
		return charmlog.DebugLevel, "Environment pulled ↓"
	case "environment.removed":
		return charmlog.ErrorLevel, "Environment removed ✕"
	case "variable.added":
		return charmlog.InfoLevel, "Variable added +"
	case "variable.changed":
		return charmlog.WarnLevel, "Variable changed ◆"
	case "variable.removed":
		return charmlog.ErrorLevel, "Variable removed −"
	default:
		return charmlog.InfoLevel, strings.ReplaceAll(action, ".", " ")
	}
}

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

func MembersTable(writer io.Writer, members []api.Member) {
	if len(members) == 0 {
		fmt.Fprintln(writer, "This project has no members.")
		return
	}
	rows := make([]table.Row, 0, len(members))
	for _, member := range members {
		rows = append(rows, table.Row{"@" + member.Username, member.Role})
	}
	renderSimpleTable(writer, []table.Column{{Title: "Member", Width: maxColumnWidth("Member", rows, 0)}, {Title: "Role", Width: maxColumnWidth("Role", rows, 1)}}, rows)
}

func InvitationsTable(writer io.Writer, invitations []api.Invitation) {
	if len(invitations) == 0 {
		fmt.Fprintln(writer, "You don't have any pending invitations.")
		return
	}
	rows := make([]table.Row, 0, len(invitations))
	for _, invitation := range invitations {
		rows = append(rows, table.Row{invitation.ID, invitation.Project, "@" + invitation.Inviter, invitation.Role, invitation.ExpiresAt.Local().Format("Jan 2")})
	}
	renderSimpleTable(writer, []table.Column{{Title: "ID", Width: maxColumnWidth("ID", rows, 0)}, {Title: "Project", Width: maxColumnWidth("Project", rows, 1)}, {Title: "From", Width: maxColumnWidth("From", rows, 2)}, {Title: "Role", Width: maxColumnWidth("Role", rows, 3)}, {Title: "Expires", Width: maxColumnWidth("Expires", rows, 4)}}, rows)
}

func maxColumnWidth(title string, rows []table.Row, index int) int {
	width := lipgloss.Width(title)
	for _, row := range rows {
		width = max(width, lipgloss.Width(row[index]))
	}
	return width + 2
}

func renderSimpleTable(writer io.Writer, columns []table.Column, rows []table.Row) {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(muted).BorderBottom(true).Bold(false).Foreground(purple)
	styles.Selected = lipgloss.NewStyle()
	model := table.New(table.WithColumns(columns), table.WithRows(rows), table.WithFocused(false), table.WithHeight(len(rows)+1), table.WithStyles(styles))
	box := lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(muted)
	fmt.Fprintln(writer, box.Render(model.View()))
}
