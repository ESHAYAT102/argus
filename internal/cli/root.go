package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/argus-env/argus/internal/api"
	"github.com/argus-env/argus/internal/config"
	"github.com/argus-env/argus/internal/dotenv"
	"github.com/argus-env/argus/internal/project"
	"github.com/argus-env/argus/internal/ui"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

const defaultAPIURL = "https://api.argus.eshayat.com"

type application struct {
	client          api.Client
	in              io.Reader
	out             io.Writer
	errOut          io.Writer
	cwd             func() (string, error)
	now             func() time.Time
	discoverProject func(string) (project.Discovery, error)
	confirmPrompt   func(string) (bool, error)
}

func Execute() error {
	baseURL := os.Getenv("ARGUS_API_URL")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	app := &application{
		client: api.NewHTTPClient(baseURL, config.SessionStore{}),
		in:     os.Stdin, out: os.Stdout, errOut: os.Stderr,
		cwd: os.Getwd, now: time.Now, discoverProject: project.Discover,
	}
	command := app.rootCommand()
	if err := command.Execute(); err != nil {
		app.printError(err)
		return err
	}
	return nil
}

func (app *application) rootCommand() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:           "argus",
		Short:         "Keep watch over your environment variables",
		SilenceErrors: true,
		SilenceUsage:  true,
		Example: `  argus auth
  argus init dev
  argus push prod
  argus pull dev
  argus set DATABASE_URL
  argus share portfolio octocat
  argus project members portfolio
  argus invites
  argus invites accept <id>`,
	}
	root.SetIn(app.in)
	root.SetOut(app.out)
	root.SetErr(app.errOut)
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddGroup(
		&cobra.Group{ID: "account", Title: "Account Commands:"},
		&cobra.Group{ID: "projects", Title: "Project & Collaboration Commands:"},
		&cobra.Group{ID: "environments", Title: "Environment Commands:"},
		&cobra.Group{ID: "variables", Title: "Variable Commands:"},
		&cobra.Group{ID: "activity", Title: "Activity Commands:"},
		&cobra.Group{ID: "help", Title: "Help Commands:"},
	)
	accountCommands := []*cobra.Command{app.authCommand(), app.whoamiCommand(), app.logoutCommand()}
	projectCommands := []*cobra.Command{app.initCommand(), app.projectCommand(), app.shareCommand(), app.listCommand(), app.invitesCommand(), app.destroyCommand()}
	environmentCommands := []*cobra.Command{app.pushCommand(), app.pullCommand(), app.statusCommand(), app.diffCommand(), app.renameEnvironmentCommand(), app.removeCommand()}
	variableCommands := []*cobra.Command{app.setCommand(), app.deleteCommand()}
	activityCommands := []*cobra.Command{app.historyCommand()}
	for _, command := range accountCommands {
		command.GroupID = "account"
	}
	for _, command := range projectCommands {
		command.GroupID = "projects"
	}
	for _, command := range environmentCommands {
		command.GroupID = "environments"
	}
	for _, command := range variableCommands {
		command.GroupID = "variables"
	}
	for _, command := range activityCommands {
		command.GroupID = "activity"
	}
	root.SetHelpCommandGroupID("help")
	root.AddCommand(accountCommands...)
	root.AddCommand(projectCommands...)
	root.AddCommand(environmentCommands...)
	root.AddCommand(variableCommands...)
	root.AddCommand(activityCommands...)
	return root
}

func (app *application) printError(err error) {
	fmt.Fprintf(app.errOut, "%s %s\n", ui.Error.Render("Error:"), err)
}

func noArgs(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return argumentError(command, fmt.Sprintf("%s doesn't accept arguments", command.Name()))
}

func atMostOneArg(command *cobra.Command, args []string) error {
	if len(args) <= 1 {
		return nil
	}
	return argumentError(command, "too many arguments: "+quotedArguments(args[1:]))
}

func requireEnvironment(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return argumentError(command, "missing environment name")
	}
	if len(args) > 1 {
		return argumentError(command, "too many arguments: "+quotedArguments(args[1:]))
	}
	return nil
}

func requireProject(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return argumentError(command, "missing project name")
	}
	if len(args) > 1 {
		return argumentError(command, "too many arguments: "+quotedArguments(args[1:]))
	}
	return nil
}

func requireVariable(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return argumentError(command, "missing variable name")
	}
	if len(args) > 1 {
		return argumentError(command, "too many arguments: "+quotedArguments(args[1:]))
	}
	return nil
}

func projectLinkArgs(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return argumentError(command, "missing project name")
	}
	if len(args) > 2 {
		return argumentError(command, "too many arguments: "+quotedArguments(args[2:]))
	}
	return nil
}

func exactArguments(count int, missing string) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if len(args) < count {
			return argumentError(command, missing)
		}
		if len(args) > count {
			return argumentError(command, "too many arguments: "+quotedArguments(args[count:]))
		}
		return nil
	}
}

func setArgs(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return argumentError(command, "missing variable name")
	}
	if len(args) > 2 {
		return argumentError(command, "too many arguments: "+quotedArguments(args[2:]))
	}
	return nil
}

func argumentError(command *cobra.Command, message string) error {
	var hint strings.Builder
	hint.WriteString(message)
	hint.WriteString("\n\nUsage:\n  ")
	hint.WriteString(command.UseLine())
	if command.Example != "" {
		hint.WriteString("\n\nExamples:\n")
		hint.WriteString(command.Example)
	}
	return errors.New(hint.String())
}

func quotedArguments(args []string) string {
	quoted := make([]string, len(args))
	for index, argument := range args {
		quoted[index] = fmt.Sprintf("%q", argument)
	}
	return strings.Join(quoted, ", ")
}

func (app *application) authCommand() *cobra.Command {
	return &cobra.Command{Use: "auth", Short: "Sign in with GitHub", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		user, err := app.client.WhoAmI(command.Context())
		if err == nil {
			fmt.Fprintf(app.out, "Already authenticated as %s.\n", user.Username)
			return nil
		}
		if !errors.Is(err, api.ErrUnauthenticated) {
			return err
		}
		if err := app.client.Authenticate(command.Context()); err != nil {
			return err
		}
		fmt.Fprintln(app.out, ui.Success.Render("Authenticated with GitHub."))
		return nil
	}}
}

func (app *application) whoamiCommand() *cobra.Command {
	return &cobra.Command{Use: "whoami", Short: "Show the authenticated GitHub username", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		user, err := app.client.WhoAmI(command.Context())
		if errors.Is(err, api.ErrUnauthenticated) {
			return errors.New("not authenticated; run `argus auth`")
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(app.out, user.Username)
		return nil
	}}
}

func (app *application) logoutCommand() *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Log out of Argus", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		if err := app.client.Logout(command.Context()); err != nil {
			return err
		}
		fmt.Fprintln(app.out, ui.Success.Render("Logged out."))
		return nil
	}}
}

func (app *application) initCommand() *cobra.Command {
	return &cobra.Command{Use: "init [environment]", Short: "Initialize a project and push its .env", Args: atMostOneArg, RunE: func(command *cobra.Command, args []string) error {
		directory, err := app.cwd()
		if err != nil {
			return err
		}
		environment, err := app.environmentArgument(args)
		if err != nil {
			return err
		}
		values, err := dotenv.Read(directory)
		if err != nil {
			return err
		}

		name, repository := "", ""
		discovered, discoveryErr := app.discover(directory)
		if discoveryErr == nil {
			name, repository, directory = discovered.Name, discovered.Repository, discovered.Root
		} else {
			fmt.Fprintln(app.out, "The current directory is not a GitHub repository.")
			name, err = app.textPrompt("Give this project a name", "")
			if err != nil {
				return err
			}
		}
		created, _, err := app.client.InitProject(command.Context(), api.InitProjectRequest{Name: name, Repository: repository, Environment: environment, Variables: values})
		if err != nil {
			return err
		}
		if err := config.Save(directory, config.Project{ProjectID: created.ID, ProjectName: created.Name, Environment: environment}); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Initialized %s with environment %s (%d variables).\n", ui.Success.Render("✓"), created.Name, environment, len(values))
		return nil
	}}
}

func (app *application) pushCommand() *cobra.Command {
	return &cobra.Command{Use: "push [environment]", Short: "Push the current .env to Argus", Args: atMostOneArg, RunE: func(command *cobra.Command, args []string) error {
		directory, metadata, err := app.projectContext(command.Context(), true)
		if err != nil {
			return err
		}
		environment := metadata.Environment
		if len(args) == 1 {
			environment = args[0]
		}
		if environment == "" {
			environment, err = app.textPrompt("Environment name", "dev")
			if err != nil {
				return err
			}
		}
		values, err := dotenv.Read(directory)
		if err != nil {
			return err
		}
		exists, err := app.client.EnvironmentExists(command.Context(), metadata.ProjectID, environment)
		if err != nil {
			return err
		}
		if !exists {
			confirmed, err := app.confirm(fmt.Sprintf("The environment %q doesn't exist. Would you like to create it?", environment))
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(app.out, "Push cancelled.")
				return nil
			}
		}
		if _, err := app.client.Push(command.Context(), metadata.ProjectID, environment, values); err != nil {
			return err
		}
		metadata.Environment = environment
		if err := config.Save(directory, metadata); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Pushed %d variables to %s.\n", ui.Success.Render("✓"), len(values), environment)
		return nil
	}}
}

func (app *application) pullCommand() *cobra.Command {
	return &cobra.Command{Use: "pull [environment]", Short: "Pull an environment into .env", Example: "  argus pull\n  argus pull dev\n  argus pull prod", Args: atMostOneArg, RunE: func(command *cobra.Command, args []string) error {
		directory, metadata, err := app.projectContext(command.Context(), true)
		if err != nil {
			return err
		}
		environment := ""
		if len(args) == 1 {
			environment = args[0]
		} else {
			environment, err = app.onlyProjectEnvironment(command.Context(), metadata)
			if err != nil {
				return err
			}
		}
		values, err := app.client.Get(command.Context(), metadata.ProjectID, environment)
		if err != nil {
			return err
		}
		managed := false
		local, localErr := dotenv.Read(directory)
		if localErr == nil && metadata.Environment != "" {
			activeRemote := values
			if !strings.EqualFold(metadata.Environment, environment) {
				activeRemote, err = app.client.Inspect(command.Context(), metadata.ProjectID, metadata.Environment)
				if err != nil && !errors.Is(err, api.ErrNotFound) {
					return err
				}
				if errors.Is(err, api.ErrNotFound) {
					activeRemote = nil
				}
			}
			managed = compareVariables(local, activeRemote).empty()
		}
		backup := ""
		if managed {
			err = dotenv.Write(directory, values)
		} else {
			backup, err = dotenv.WriteSafely(directory, values, app.now())
		}
		if err != nil {
			return err
		}
		metadata.Environment = environment
		if err := config.Save(directory, metadata); err != nil {
			return err
		}
		if backup != "" {
			fmt.Fprintf(app.out, "Backup created: %s\n", filepath.Base(backup))
		}
		fmt.Fprintf(app.out, "%s Pulled %s from %s. %d variables written to .env.\n", ui.Success.Render("✓"), environment, metadata.ProjectName, len(values))
		return nil
	}}
}

func (app *application) onlyProjectEnvironment(ctx context.Context, metadata config.Project) (string, error) {
	projects, err := app.client.List(ctx)
	if err != nil {
		return "", err
	}
	for _, candidate := range projects {
		if candidate.ID != metadata.ProjectID {
			continue
		}
		switch len(candidate.Environments) {
		case 0:
			return "", fmt.Errorf("project %q has no environments", metadata.ProjectName)
		case 1:
			return candidate.Environments[0].Name, nil
		default:
			names := make([]string, 0, len(candidate.Environments))
			for _, environment := range candidate.Environments {
				names = append(names, environment.Name)
			}
			sort.Strings(names)
			return "", fmt.Errorf("project %q has multiple environments; specify one: %s", metadata.ProjectName, strings.Join(names, ", "))
		}
	}
	return "", fmt.Errorf("project %q was not found", metadata.ProjectName)
}

func (app *application) setCommand() *cobra.Command {
	return &cobra.Command{Use: "set <variable> [value]", Short: "Set a variable locally and remotely", Example: "  argus set API_KEY\n  argus set PORT 3000", Args: setArgs, RunE: func(command *cobra.Command, args []string) error {
		if err := dotenv.ValidateName(args[0]); err != nil {
			return err
		}
		directory, metadata, err := app.projectContext(command.Context(), false)
		if err != nil {
			return err
		}
		if metadata.Environment == "" {
			return errors.New("no active environment; run `argus pull <environment>` or `argus push <environment>` first")
		}
		value := ""
		if len(args) == 2 {
			value = args[1]
		} else {
			value, err = app.secretPrompt("Value")
			if err != nil {
				return err
			}
		}
		// Update the remote first so a failed network request does not create local drift.
		if err := app.client.Set(command.Context(), metadata.ProjectID, metadata.Environment, args[0], value); err != nil {
			return err
		}
		if err := dotenv.Set(directory, args[0], value); err != nil {
			return fmt.Errorf("remote value changed, but local .env update failed: %w", err)
		}
		fmt.Fprintf(app.out, "%s Set %s in %s.\n", ui.Success.Render("✓"), args[0], metadata.Environment)
		return nil
	}}
}

type variableDiff struct {
	LocalOnly  []string
	Changed    []string
	RemoteOnly []string
}

func compareVariables(local, remote map[string]string) variableDiff {
	var result variableDiff
	for name, localValue := range local {
		remoteValue, exists := remote[name]
		switch {
		case !exists:
			result.LocalOnly = append(result.LocalOnly, name)
		case localValue != remoteValue:
			result.Changed = append(result.Changed, name)
		}
	}
	for name := range remote {
		if _, exists := local[name]; !exists {
			result.RemoteOnly = append(result.RemoteOnly, name)
		}
	}
	sort.Strings(result.LocalOnly)
	sort.Strings(result.Changed)
	sort.Strings(result.RemoteOnly)
	return result
}

func (difference variableDiff) empty() bool {
	return len(difference.LocalOnly) == 0 && len(difference.Changed) == 0 && len(difference.RemoteOnly) == 0
}

func (app *application) statusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show the current project and environment status", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		directory, metadata, err := app.projectContext(command.Context(), true)
		if err != nil {
			return err
		}
		if metadata.Environment == "" {
			return errors.New("no active environment; run `argus pull <environment>` or `argus push <environment>` first")
		}
		local, err := dotenv.Read(directory)
		if err != nil {
			return err
		}
		remote, err := app.client.Inspect(command.Context(), metadata.ProjectID, metadata.Environment)
		if err != nil {
			return err
		}
		difference := compareVariables(local, remote)
		fmt.Fprintf(app.out, "%s %s\n%s %s\n", ui.Muted.Render("Project:"), metadata.ProjectName, ui.Muted.Render("Environment:"), metadata.Environment)
		if difference.empty() {
			fmt.Fprintln(app.out, ui.Success.Render("✓ Local .env is in sync."))
			return nil
		}
		fmt.Fprintf(app.out, "%s Local .env differs: %d local-only, %d changed, %d remote-only.\n", ui.Error.Render("!"), len(difference.LocalOnly), len(difference.Changed), len(difference.RemoteOnly))
		return nil
	}}
}

func (app *application) diffCommand() *cobra.Command {
	return &cobra.Command{Use: "diff <environment>", Short: "Compare local .env with a remote environment", Example: "  argus diff dev\n  argus diff prod", Args: requireEnvironment, RunE: func(command *cobra.Command, args []string) error {
		directory, metadata, err := app.projectContext(command.Context(), true)
		if err != nil {
			return err
		}
		local, err := dotenv.Read(directory)
		if err != nil {
			return err
		}
		remote, err := app.client.Inspect(command.Context(), metadata.ProjectID, args[0])
		if err != nil {
			return err
		}
		difference := compareVariables(local, remote)
		if difference.empty() {
			fmt.Fprintf(app.out, "%s Local .env matches %s.\n", ui.Success.Render("✓"), args[0])
			return nil
		}
		for _, name := range difference.LocalOnly {
			fmt.Fprintf(app.out, "%s %s %s\n", ui.Success.Render("+"), name, ui.Muted.Render("local only"))
		}
		for _, name := range difference.Changed {
			fmt.Fprintf(app.out, "%s %s %s\n", ui.Title.Render("~"), name, ui.Muted.Render("changed"))
		}
		for _, name := range difference.RemoteOnly {
			fmt.Fprintf(app.out, "%s %s %s\n", ui.Error.Render("-"), name, ui.Muted.Render("remote only"))
		}
		return nil
	}}
}

func (app *application) deleteCommand() *cobra.Command {
	return &cobra.Command{Use: "delete <variable>", Short: "Delete a variable locally and remotely", Example: "  argus delete OLD_API_KEY", Args: requireVariable, RunE: func(command *cobra.Command, args []string) error {
		if err := dotenv.ValidateName(args[0]); err != nil {
			return err
		}
		directory, metadata, err := app.projectContext(command.Context(), false)
		if err != nil {
			return err
		}
		if metadata.Environment == "" {
			return errors.New("no active environment; run `argus pull <environment>` or `argus push <environment>` first")
		}
		confirmed, err := app.confirm(fmt.Sprintf("Delete %q from %q? This removes it locally and remotely.", args[0], metadata.Environment))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(app.out, "Deletion cancelled.")
			return nil
		}
		if err := app.client.DeleteVariable(command.Context(), metadata.ProjectID, metadata.Environment, args[0]); err != nil {
			return err
		}
		if _, err := dotenv.Delete(directory, args[0]); err != nil {
			return fmt.Errorf("remote variable was deleted, but local .env update failed: %w", err)
		}
		fmt.Fprintf(app.out, "%s Deleted %s from %s.\n", ui.Success.Render("✓"), args[0], metadata.Environment)
		return nil
	}}
}

func (app *application) projectCommand() *cobra.Command {
	command := &cobra.Command{Use: "project", Short: "Link and share projects; manage members"}
	command.AddCommand(app.projectLinkCommand(), app.projectRenameCommand(), app.projectShareCommand(), app.projectMembersCommand(), app.projectRoleCommand(), app.projectUnshareCommand())
	return command
}

func (app *application) projectRenameCommand() *cobra.Command {
	return &cobra.Command{Use: "rename <project> <new-name>", Short: "Rename a project", Example: "  argus project rename portfolio website", Args: exactArguments(2, "project and new name are required"), RunE: func(command *cobra.Command, args []string) error {
		newName := strings.TrimSpace(args[1])
		if newName == "" {
			return errors.New("new project name is required")
		}
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		if err := app.client.RenameProject(command.Context(), metadata.ProjectID, newName); err != nil {
			return err
		}
		if err := config.RenameProject(metadata.ProjectID, newName); err != nil {
			return fmt.Errorf("project was renamed remotely, but local registry update failed: %w", err)
		}
		fmt.Fprintf(app.out, "%s Renamed project %s to %s.\n", ui.Success.Render("✓"), metadata.ProjectName, newName)
		return nil
	}}
}

func (app *application) renameEnvironmentCommand() *cobra.Command {
	return &cobra.Command{Use: "rename <environment> <new-name>", Short: "Rename an environment", Example: "  argus rename staging preview", Args: exactArguments(2, "environment and new name are required"), RunE: func(command *cobra.Command, args []string) error {
		newName := strings.TrimSpace(args[1])
		if newName == "" {
			return errors.New("new environment name is required")
		}
		_, metadata, err := app.projectContext(command.Context(), false)
		if err != nil {
			return err
		}
		if err := app.client.RenameEnvironment(command.Context(), metadata.ProjectID, args[0], newName); err != nil {
			return err
		}
		if err := config.RenameEnvironment(metadata.ProjectID, args[0], newName); err != nil {
			return fmt.Errorf("environment was renamed remotely, but local registry update failed: %w", err)
		}
		fmt.Fprintf(app.out, "%s Renamed environment %s to %s.\n", ui.Success.Render("✓"), args[0], newName)
		return nil
	}}
}

func (app *application) projectShareCommand() *cobra.Command {
	return app.newShareCommand("share <project> <github-user>")
}

func (app *application) shareCommand() *cobra.Command {
	return app.newShareCommand("share <project> <github-user>")
}

func (app *application) newShareCommand(use string) *cobra.Command {
	role := ""
	command := &cobra.Command{Use: use, Short: "Invite a GitHub user to a project", Example: "  argus share portfolio octocat\n  argus share portfolio octocat --role viewer", Args: exactArguments(2, "project name and GitHub username are required"), RunE: func(command *cobra.Command, args []string) error {
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		selectedRole := strings.ToLower(role)
		if selectedRole == "" {
			selectedRole, err = app.selectRole()
			if err != nil {
				return err
			}
		}
		if !validMemberRole(selectedRole) {
			return errors.New("role must be admin, member, or viewer")
		}
		username := strings.TrimPrefix(args[1], "@")
		invitation, err := app.client.ShareProject(command.Context(), metadata.ProjectID, username, selectedRole)
		if err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Invited @%s to %s as %s. Invitation expires %s.\n", ui.Success.Render("✓"), username, metadata.ProjectName, invitation.Role, invitation.ExpiresAt.Local().Format("Jan 2"))
		return nil
	}}
	command.Flags().StringVar(&role, "role", "", "access role: admin, member, or viewer")
	return command
}

func (app *application) projectMembersCommand() *cobra.Command {
	return &cobra.Command{Use: "members <project>", Short: "List project members", Args: requireProject, RunE: func(command *cobra.Command, args []string) error {
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		members, err := app.client.Members(command.Context(), metadata.ProjectID)
		if err != nil {
			return err
		}
		ui.MembersTable(app.out, members)
		return nil
	}}
}

func (app *application) projectRoleCommand() *cobra.Command {
	return &cobra.Command{Use: "role <project> <github-user> <role>", Short: "Change a project member's role", Example: "  argus project role portfolio octocat viewer", Args: exactArguments(3, "project, GitHub username, and role are required"), RunE: func(command *cobra.Command, args []string) error {
		role := strings.ToLower(args[2])
		if !validMemberRole(role) {
			return errors.New("role must be admin, member, or viewer")
		}
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		username := strings.TrimPrefix(args[1], "@")
		if err := app.client.UpdateMemberRole(command.Context(), metadata.ProjectID, username, role); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Changed @%s to %s in %s.\n", ui.Success.Render("✓"), username, role, metadata.ProjectName)
		return nil
	}}
}

func (app *application) projectUnshareCommand() *cobra.Command {
	return &cobra.Command{Use: "unshare <project> <github-user>", Short: "Remove a user's project access", Args: exactArguments(2, "project name and GitHub username are required"), RunE: func(command *cobra.Command, args []string) error {
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		username := strings.TrimPrefix(args[1], "@")
		confirmed, err := app.confirm(fmt.Sprintf("Remove @%s from %q? They will immediately lose access.", username, metadata.ProjectName))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(app.out, "Access removal cancelled.")
			return nil
		}
		if err := app.client.RemoveMember(command.Context(), metadata.ProjectID, username); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Removed @%s from %s.\n", ui.Success.Render("✓"), username, metadata.ProjectName)
		return nil
	}}
}

func (app *application) invitesCommand() *cobra.Command {
	command := &cobra.Command{Use: "invites", Short: "View project invitations", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		invitations, err := app.client.Invitations(command.Context())
		if err != nil {
			return err
		}
		ui.InvitationsTable(app.out, invitations)
		return nil
	}}
	command.AddCommand(
		&cobra.Command{Use: "accept <id>", Short: "Accept a project invitation", Args: exactArguments(1, "invitation id is required"), RunE: func(command *cobra.Command, args []string) error {
			if err := app.client.AcceptInvitation(command.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(app.out, ui.Success.Render("✓ Invitation accepted."))
			return nil
		}},
		&cobra.Command{Use: "decline <id>", Short: "Decline a project invitation", Args: exactArguments(1, "invitation id is required"), RunE: func(command *cobra.Command, args []string) error {
			if err := app.client.DeclineInvitation(command.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(app.out, "Invitation declined.")
			return nil
		}},
	)
	return command
}

func (app *application) selectRole() (string, error) {
	value := "member"
	err := huh.NewSelect[string]().Title("Role").Options(
		huh.NewOption("Member — manage environments and variables", "member"),
		huh.NewOption("Viewer — read only", "viewer"),
		huh.NewOption("Admin — manage members and variables", "admin"),
	).Value(&value).Run()
	return value, err
}

func validMemberRole(role string) bool {
	return role == "admin" || role == "member" || role == "viewer"
}

func (app *application) projectLinkCommand() *cobra.Command {
	return &cobra.Command{Use: "link <project> [environment]", Short: "Link the current directory to an existing project", Example: "  argus project link portfolio\n  argus project link portfolio prod", Args: projectLinkArgs, RunE: func(command *cobra.Command, args []string) error {
		directory, err := app.cwd()
		if err != nil {
			return err
		}
		if discovered, discoveryErr := app.discover(directory); discoveryErr == nil {
			directory = discovered.Root
		}
		projects, err := app.client.List(command.Context())
		if err != nil {
			return err
		}
		var selected *api.Project
		for index := range projects {
			if strings.EqualFold(projects[index].Name, args[0]) {
				selected = &projects[index]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("project %q was not found", args[0])
		}
		environment := ""
		if len(args) == 2 {
			environment, _ = projectEnvironment(*selected, args[1])
			if environment == "" {
				return fmt.Errorf("environment %q was not found in project %q", args[1], selected.Name)
			}
		} else if len(selected.Environments) == 1 {
			environment = selected.Environments[0].Name
		} else if len(selected.Environments) > 1 {
			environment, err = app.selectEnvironment(selected.Environments)
			if err != nil {
				return err
			}
		}
		if err := config.Save(directory, config.Project{ProjectID: selected.ID, ProjectName: selected.Name, Environment: environment}); err != nil {
			return err
		}
		if environment == "" {
			fmt.Fprintf(app.out, "%s Linked %s to %s.\n", ui.Success.Render("✓"), directory, selected.Name)
		} else {
			fmt.Fprintf(app.out, "%s Linked %s to %s (%s).\n", ui.Success.Render("✓"), directory, selected.Name, environment)
		}
		return nil
	}}
}

func projectEnvironment(project api.Project, name string) (string, bool) {
	for _, environment := range project.Environments {
		if strings.EqualFold(environment.Name, name) {
			return environment.Name, true
		}
	}
	return "", false
}

func (app *application) selectEnvironment(environments []api.Environment) (string, error) {
	options := make([]huh.Option[string], 0, len(environments))
	for _, environment := range environments {
		options = append(options, huh.NewOption(environment.Name, environment.Name))
	}
	value := ""
	if err := huh.NewSelect[string]().Title("Environment").Options(options...).Value(&value).Run(); err != nil {
		return "", err
	}
	return value, nil
}

func (app *application) listCommand() *cobra.Command {
	command := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List projects and environments", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		projects, err := app.client.List(command.Context())
		if err != nil {
			return err
		}
		if len(projects) == 0 {
			fmt.Fprintln(app.out, "You don't have any projects yet.")
			return nil
		}
		rows := make([]struct{ Name, Environments string }, 0, len(projects))
		for _, item := range projects {
			names := make([]string, 0, len(item.Environments))
			for _, environment := range item.Environments {
				names = append(names, environment.Name)
			}
			sort.Strings(names)
			rows = append(rows, struct{ Name, Environments string }{item.Name, strings.Join(names, ", ")})
		}
		ui.ProjectsTable(app.out, rows)
		return nil
	}}
	return command
}

func (app *application) historyCommand() *cobra.Command {
	return &cobra.Command{Use: "history", Aliases: []string{"activity"}, Short: "Show project activity", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		projectID := ""
		if directory, err := app.cwd(); err == nil {
			if metadata, err := config.Load(directory); err == nil {
				projectID = metadata.ProjectID
			} else if discovered, discoveryErr := app.discover(directory); discoveryErr == nil {
				if metadata, loadErr := config.Load(discovered.Root); loadErr == nil {
					projectID = metadata.ProjectID
				}
			}
		}
		activity, err := app.client.History(command.Context(), projectID)
		if err != nil {
			return err
		}
		ui.ActivityLog(app.out, activity)
		return nil
	}}
}

func (app *application) removeCommand() *cobra.Command {
	return &cobra.Command{Use: "remove <environment>", Aliases: []string{"rm"}, Short: "Permanently remove an environment", Example: "  argus remove dev\n  argus rm prod", Args: requireEnvironment, RunE: func(command *cobra.Command, args []string) error {
		_, metadata, err := app.projectContext(command.Context(), false)
		if err != nil {
			return err
		}
		confirmed, err := app.confirm(fmt.Sprintf("Remove environment %q from %q? This action cannot be undone.", args[0], metadata.ProjectName))
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(app.out, "Removal cancelled.")
			return nil
		}
		if err := app.client.RemoveEnvironment(command.Context(), metadata.ProjectID, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Removed environment %s.\n", ui.Success.Render("✓"), args[0])
		return nil
	}}
}

func (app *application) destroyCommand() *cobra.Command {
	return &cobra.Command{Use: "destroy <project>", Short: "Permanently destroy a project", Example: "  argus destroy portfolio", Args: requireProject, RunE: func(command *cobra.Command, args []string) error {
		metadata, err := app.destroyTarget(command.Context(), args[0])
		if err != nil {
			return err
		}
		name := metadata.ProjectName
		answer, err := app.textPrompt(fmt.Sprintf("Type %q to confirm permanent destruction", name), "")
		if err != nil {
			return err
		}
		if answer != name {
			return errors.New("destroy cancelled: project name did not match")
		}
		if err := app.client.DestroyProject(command.Context(), metadata.ProjectID); err != nil {
			return err
		}
		if err := config.RemoveProject(metadata.ProjectID); err != nil {
			return fmt.Errorf("project was destroyed, but its local registry entries could not be removed: %w", err)
		}
		fmt.Fprintf(app.out, "%s Destroyed project %s.\n", ui.Success.Render("✓"), name)
		return nil
	}}
}

func (app *application) destroyTarget(ctx context.Context, name string) (config.Project, error) {
	projects, err := app.client.List(ctx)
	if err != nil {
		return config.Project{}, err
	}
	for _, candidate := range projects {
		if strings.EqualFold(candidate.Name, name) {
			return config.Project{ProjectID: candidate.ID, ProjectName: candidate.Name}, nil
		}
	}
	return config.Project{}, fmt.Errorf("project %q was not found", name)
}

func (app *application) projectContext(ctx context.Context, allowLookup bool) (string, config.Project, error) {
	directory, err := app.cwd()
	if err != nil {
		return "", config.Project{}, err
	}
	metadata, err := config.Load(directory)
	if err == nil {
		return directory, metadata, nil
	}
	if discovered, discoveryErr := app.discover(directory); discoveryErr == nil {
		metadata, err = config.Load(discovered.Root)
		if err == nil {
			return discovered.Root, metadata, nil
		}
		if allowLookup {
			projects, listErr := app.client.List(ctx)
			if listErr != nil {
				return "", config.Project{}, listErr
			}
			for _, candidate := range projects {
				if strings.EqualFold(candidate.Repository, discovered.Repository) {
					metadata = config.Project{ProjectID: candidate.ID, ProjectName: candidate.Name}
					if saveErr := config.Save(discovered.Root, metadata); saveErr != nil {
						return "", config.Project{}, saveErr
					}
					return discovered.Root, metadata, nil
				}
			}
			return "", config.Project{}, fmt.Errorf("GitHub repository %q is not initialized in Argus; run `argus init` first", discovered.Repository)
		}
	}
	if allowLookup {
		fmt.Fprintln(app.out, "The current directory is not a GitHub repository or registered Argus project.")
		name, promptErr := app.textPrompt("Project name", "")
		if promptErr != nil {
			return "", config.Project{}, promptErr
		}
		projects, listErr := app.client.List(ctx)
		if listErr != nil {
			return "", config.Project{}, listErr
		}
		for _, candidate := range projects {
			if candidate.Name == name {
				metadata = config.Project{ProjectID: candidate.ID, ProjectName: candidate.Name}
				if saveErr := config.Save(directory, metadata); saveErr != nil {
					return "", config.Project{}, saveErr
				}
				return directory, metadata, nil
			}
		}
		return "", config.Project{}, fmt.Errorf("project %q was not found", name)
	}
	return "", config.Project{}, errors.New("directory is not connected to Argus; run `argus init` first")
}

func (app *application) discover(directory string) (project.Discovery, error) {
	if app.discoverProject != nil {
		return app.discoverProject(directory)
	}
	return project.Discover(directory)
}

func (app *application) environmentArgument(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return app.textPrompt("Environment name", "dev")
}

func (app *application) textPrompt(title, placeholder string) (string, error) {
	value := ""
	field := huh.NewInput().Title(title).Value(&value)
	if placeholder != "" {
		field.Placeholder(placeholder)
	}
	if err := field.Run(); err != nil {
		return "", err
	}
	if value == "" {
		value = placeholder
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("a value is required")
	}
	return strings.TrimSpace(value), nil
}

func (app *application) secretPrompt(title string) (string, error) {
	value := ""
	if err := huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(&value).Run(); err != nil {
		return "", err
	}
	return value, nil
}

func (app *application) confirm(title string) (bool, error) {
	if app.confirmPrompt != nil {
		return app.confirmPrompt(title)
	}
	confirmed := false
	err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		WithButtonAlignment(lipgloss.Left).
		Value(&confirmed).
		Run()
	return confirmed, err
}
