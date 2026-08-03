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
	"github.com/spf13/cobra"
)

const defaultAPIURL = "https://api.argus.eshayat.com"

type application struct {
	client api.Client
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	cwd    func() (string, error)
	now    func() time.Time
}

func Execute() error {
	baseURL := os.Getenv("ARGUS_API_URL")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	app := &application{
		client: api.NewHTTPClient(baseURL, config.SessionStore{}),
		in:     os.Stdin, out: os.Stdout, errOut: os.Stderr,
		cwd: os.Getwd, now: time.Now,
	}
	command := app.rootCommand()
	if err := command.Execute(); err != nil {
		app.printError(err)
		return err
	}
	return nil
}

func (app *application) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "argus",
		Short:         "Keep watch over your environment variables",
		SilenceErrors: true,
		SilenceUsage:  true,
		Example: `  argus auth
  argus init dev
  argus sync prod
  argus get dev
  argus set DATABASE_URL`,
	}
	root.SetIn(app.in)
	root.SetOut(app.out)
	root.SetErr(app.errOut)
	root.AddCommand(
		app.authCommand(), app.logoutCommand(), app.initCommand(),
		app.syncCommand(), app.getCommand(), app.setCommand(),
		app.listCommand(), app.historyCommand(), app.removeCommand(),
		app.destroyCommand(),
	)
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
		if err := app.client.Authenticate(command.Context()); err != nil {
			return err
		}
		fmt.Fprintln(app.out, ui.Success.Render("Authenticated with GitHub."))
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
		discovered, discoveryErr := project.Discover(directory)
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

func (app *application) syncCommand() *cobra.Command {
	return &cobra.Command{Use: "sync [environment]", Short: "Push the current .env", Args: atMostOneArg, RunE: func(command *cobra.Command, args []string) error {
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
				fmt.Fprintln(app.out, "Sync cancelled.")
				return nil
			}
		}
		if _, err := app.client.Sync(command.Context(), metadata.ProjectID, environment, values); err != nil {
			return err
		}
		metadata.Environment = environment
		if err := config.Save(directory, metadata); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "%s Synced %d variables to %s.\n", ui.Success.Render("✓"), len(values), environment)
		return nil
	}}
}

func (app *application) getCommand() *cobra.Command {
	return &cobra.Command{Use: "get <environment>", Short: "Fetch an environment into .env", Example: "  argus get dev\n  argus get prod", Args: requireEnvironment, RunE: func(command *cobra.Command, args []string) error {
		directory, metadata, err := app.projectContext(command.Context(), true)
		if err != nil {
			return err
		}
		values, err := app.client.Get(command.Context(), metadata.ProjectID, args[0])
		if err != nil {
			return err
		}
		backup, err := dotenv.WriteSafely(directory, values, app.now())
		if err != nil {
			return err
		}
		metadata.Environment = args[0]
		if err := config.Save(directory, metadata); err != nil {
			return err
		}
		if backup != "" {
			fmt.Fprintf(app.out, "Backup created: %s\n", filepath.Base(backup))
		}
		fmt.Fprintf(app.out, "%s Fetched %s from %s. %d variables written to .env.\n", ui.Success.Render("✓"), args[0], metadata.ProjectName, len(values))
		return nil
	}}
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
			return errors.New("no active environment; run `argus get <environment>` or `argus sync <environment>` first")
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

func (app *application) listCommand() *cobra.Command {
	command := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List projects and environments", Args: noArgs, RunE: func(command *cobra.Command, _ []string) error {
		projects, err := app.client.List(command.Context())
		if err != nil {
			return err
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
			}
		}
		activity, err := app.client.History(command.Context(), projectID)
		if err != nil {
			return err
		}
		for _, event := range activity {
			details := event.Environment
			if event.Variable != "" {
				details += " " + event.Variable
			}
			fmt.Fprintf(app.out, "%-20s %-18s %-16s %s\n", event.CreatedAt.Local().Format("2006-01-02 15:04"), event.Actor, event.Action, strings.TrimSpace(details))
		}
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
	return &cobra.Command{Use: "destroy [project]", Short: "Permanently destroy a project", Args: atMostOneArg, RunE: func(command *cobra.Command, args []string) error {
		directory, metadata, err := app.destroyTarget(command.Context(), args)
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
		if directory != "" {
			_ = os.Remove(config.Path(directory))
		}
		fmt.Fprintf(app.out, "%s Destroyed project %s.\n", ui.Success.Render("✓"), name)
		return nil
	}}
}

func (app *application) destroyTarget(ctx context.Context, args []string) (string, config.Project, error) {
	if len(args) == 0 {
		return app.projectContext(ctx, false)
	}
	projects, err := app.client.List(ctx)
	if err != nil {
		return "", config.Project{}, err
	}
	for _, candidate := range projects {
		if candidate.Name == args[0] {
			return "", config.Project{ProjectID: candidate.ID, ProjectName: candidate.Name}, nil
		}
	}
	return "", config.Project{}, fmt.Errorf("project %q was not found", args[0])
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
	if discovered, discoveryErr := project.Discover(directory); discoveryErr == nil {
		metadata, err = config.Load(discovered.Root)
		if err == nil {
			return discovered.Root, metadata, nil
		}
	}
	if allowLookup {
		fmt.Fprintln(app.out, "This directory is not connected to an Argus project.")
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
	confirmed := false
	err := huh.NewConfirm().Title(title).Affirmative("Yes").Negative("No").Value(&confirmed).Run()
	return confirmed, err
}
