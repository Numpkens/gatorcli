package main

import (
	"context" // New import for the HTTP request context
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Numpkens/gatorcli/internal/config"
	"github.com/Numpkens/gatorcli/internal/feed" // New import for the feed package
)

// state holds the application's current state, including a pointer to the configuration.
type state struct {
	Config *config.Config
}

// command represents the data parsed from the command-line arguments.
type command struct {
	Name string
	Args []string
}

// commandHandlerFunc defines the signature for all command handler functions.
type commandHandlerFunc func(s *state, cmd command) error

// commands holds the map of command names to their handler functions.
type commands struct {
	Handlers map[string]commandHandlerFunc
}

// handlerLogin is the function that handles the 'gator login <username>' command.
func handlerLogin(s *state, cmd command) error {
	// 1. Check if the required argument (username) is provided.
	if len(cmd.Args) == 0 {
		return errors.New("login command requires a single argument: <username>")
	}

	// The username is the first argument in the Args slice.
	username := cmd.Args[0]

	// 2. Use the state's access to the Config struct to set the user.
	if err := s.Config.SetUser(username); err != nil {
		return fmt.Errorf("failed to set current user: %w", err)
	}

	// 3. Print success message.
	fmt.Printf("Current user has been successfully set to: %s\n", username)

	return nil
}

// handlerAgg handles the 'gator agg' command, fetching a single feed.
func handlerAgg(s *state, cmd command) error {
	// Define the feed URL for testing.
	feedURL := "https://www.wagslane.dev/index.xml"

	// Use context.Background() for simplicity for a single, immediate request.
	ctx := context.Background()

	fmt.Printf("Fetching feed from: %s\n", feedURL)

	// Call the FetchFeed function from the internal/feed package.
	rssFeed, err := feed.FetchFeed(ctx, feedURL)
	if err != nil {
		return fmt.Errorf("agg command failed to fetch feed: %w", err)
	}

	// Print the entire struct to the console (using %+v to show field names).
	fmt.Printf("\nSuccessfully fetched and parsed feed. Content:\n%+v\n", rssFeed)

	return nil
}

// run executes a registered command given the command name.
func (c *commands) run(s *state, cmd command) error {
	handler, ok := c.Handlers[cmd.Name]
	if !ok {
		// Command not found
		return fmt.Errorf("unknown command: %s", cmd.Name)
	}

	// Execute the registered handler function.
	return handler(s, cmd)
}

// register adds a new handler function for a specific command name.
func (c *commands) register(name string, f commandHandlerFunc) {
	c.Handlers[name] = f
}

func main() {
	// --- 1. State Initialization (Reading Config) ---
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Error reading initial config: %v", err)
	}
	// Create a new instance of the state struct with a pointer to the config.
	appState := &state{
		Config: &cfg,
	}

	// --- 2. Command Registration ---
	// Create a new instance of the commands struct with an initialized map.
	cmdRegistry := &commands{
		Handlers: make(map[string]commandHandlerFunc),
	}
	// Register command handlers.
	cmdRegistry.register("login", handlerLogin)
	cmdRegistry.register("agg", handlerAgg) // Registration for the new 'agg' command

	// --- 3. Argument Handling and Execution ---
	// os.Args contains all command line arguments, including the program name.
	args := os.Args

	// Check for minimum arguments: program_name (0) + command_name (1) = 2
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: Not enough arguments provided. Command name is required.")
		// Exit with code 1 to indicate an error.
		os.Exit(1)
	}

	// The command name is the second argument (index 1).
	commandName := strings.ToLower(args[1])

	// The command arguments are the remaining arguments (index 2 onwards).
	commandArgs := args[2:]

	// Create the command instance.
	currentCommand := command{
		Name: commandName,
		Args: commandArgs,
	}

	// Run the command and handle any errors.
	if err := cmdRegistry.run(appState, currentCommand); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command '%s': %v\n", commandName, err)
		// Exit with code 1 if the command failed.
		os.Exit(1)
	}
}
