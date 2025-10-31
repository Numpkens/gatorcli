package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Numpkens/gatorcli/internal/config"
)

type state struct {
	Config *config.Config
}

type command struct {
	Name string
	Args []string
}

type commandHandlerFunc func(s *state, cmd command) error

type commands struct {
	Handlers map[string]commandHandlerFunc
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.Args) == 0 {
		return errors.New("login command requires a single argument: <username>")
	}

	username := cmd.Args[0]

	if err := s.Config.SetUser(username); err != nil {
		return fmt.Errorf("failed to set current user: %w", err)
	}

	fmt.Printf("Current user has been successfully set to: %s\n", username)

	return nil
}

func (c *commands) run(s *state, cmd command) error {
	handler, ok := c.Handlers[cmd.Name]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmd.Name)
	}

	return handler(s, cmd)
}

func (c *commands) register(name string, f commandHandlerFunc) {
	c.Handlers[name] = f
}

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Error reading initial config: %v", err)
	}
	appState := &state{
		Config: &cfg,
	}

	cmdRegistry := &commands{
		Handlers: make(map[string]commandHandlerFunc),
	}
	cmdRegistry.register("login", handlerLogin)

	args := os.Args

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: Not enough arguments provided. Command name is required.")
		os.Exit(1)
	}

	commandName := strings.ToLower(args[1])

	commandArgs := args[2:]

	currentCommand := command{
		Name: commandName,
		Args: commandArgs,
	}

	if err := cmdRegistry.run(appState, currentCommand); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command '%s': %v\n", commandName, err)
		os.Exit(1)
	}
}
