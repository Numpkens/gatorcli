package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq" // Registers the "postgres" driver

	"github.com/google/uuid"

	"github.com/Numpkens/gatorcli/internal/config"
	"github.com/Numpkens/gatorcli/internal/database"
	"github.com/Numpkens/gatorcli/internal/feed"
)

// state holds the application's current state, including pointers to config and database query objects.
type state struct {
	Config *config.Config
	DB     *database.Queries
	DBConn *sql.DB // Added to allow raw SQL commands like TRUNCATE
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

func (c *commands) register(name string, handler commandHandlerFunc) {
	c.Handlers[name] = handler
}

func (c *commands) run(s *state, cmd command) error {
	handler, ok := c.Handlers[cmd.Name]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmd.Name)
	}
	return handler(s, cmd)
}

// --- COMMAND HANDLERS ---

func handlerReset(s *state, cmd command) error {
	ctx := context.Background()

	_, err := s.DBConn.ExecContext(ctx, "TRUNCATE TABLE users RESTART IDENTITY CASCADE;")
	if err != nil {
		return fmt.Errorf("failed to reset database tables: %w", err)
	}

	fmt.Println("Database reset initiated.")
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.Args) == 0 {
		return errors.New("register command requires a single argument: <username>")
	}

	username := cmd.Args[0]
	userID := uuid.New()
	now := time.Now().UTC()

	_, err := s.DB.CreateUser(context.Background(), database.CreateUserParams{
		ID:        userID,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      username,
	})
	if err != nil {
		return fmt.Errorf("failed to create user in database: %w", err)
	}

	// 2. Set the user ID in the config for the current session
	if err := s.Config.SetUser(userID.String()); err != nil {
		return fmt.Errorf("failed to register and set current user: %w", err)
	}

	fmt.Printf("User %s registered successfully (ID: %s).\n", username, userID.String())
	return nil
}

// handlerLogin is now primarily for setting a user, using a placeholder ID if needed.
func handlerLogin(s *state, cmd command) error {
	if len(cmd.Args) == 0 {
		return errors.New("login command requires a single argument: <username>")
	}

	username := cmd.Args[0]
	// In a real app, you would fetch the user ID from the database here.
	placeholderUserID := uuid.New().String()

	if err := s.Config.SetUser(placeholderUserID); err != nil {
		return fmt.Errorf("failed to set current user: %w", err)
	}

	fmt.Printf("Successfully set current user to: %s (ID: %s)\n", username, placeholderUserID)

	return nil
}

func handlerAgg(s *state, cmd command) error {
	const testFeedURL = "https://www.freecodecamp.org/news/rss/"

	fmt.Printf("Fetching test feed: %s\n", testFeedURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rssFeed, err := feed.FetchFeed(ctx, testFeedURL)
	if err != nil {
		return fmt.Errorf("error fetching feed: %w", err)
	}

	fmt.Printf("Feed fetched successfully: %s\n", rssFeed.Channel.Title)
	fmt.Printf("Found %d items.\n", len(rssFeed.Channel.Item))

	return nil
}

func handlerAddFeed(s *state, cmd command) error {
	if len(cmd.Args) != 2 {
		return errors.New("addfeed command requires two arguments: <name> <url>")
	}
	feedName := cmd.Args[0]
	feedURL := cmd.Args[1]

	if s.Config.UserID == "" {
		return errors.New("user is not logged in. Please run 'gator login <username>' first")
	}

	userID, err := uuid.Parse(s.Config.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID in config: %w", err)
	}

	now := time.Now().UTC()

	newFeed, err := s.DB.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      feedName,
		Url:       feedURL,
		UserID:    userID,
	})

	if err != nil {
		return fmt.Errorf("failed to create feed in database: %w", err)
	}

	fmt.Printf("Successfully added new feed:\n")
	fmt.Printf("  ID:        %s\n", newFeed.ID)
	fmt.Printf("  Name:      %s\n", newFeed.Name)
	fmt.Printf("  URL:       %s\n", newFeed.Url)
	fmt.Printf("  User ID:   %s\n", newFeed.UserID)
	fmt.Printf("  Created At: %s\n", newFeed.CreatedAt)

	return nil
}

func main() {
	// --- 1. State Initialization (Reading Config and DB Connection) ---
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Error reading initial config: %v", err)
	}

	// DB connection setup
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "host=localhost port=5432 user=postgres password=postgres dbname=gatorcli sslmode=disable"
	}

	dbConn, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database connection: %v", err)
	}
	defer dbConn.Close()

	dbQueries := database.New(dbConn)

	appState := &state{
		Config: &cfg,
		DB:     dbQueries,
		DBConn: dbConn, // Pass the raw connection for TRUNCATE
	}

	// --- 2. Command Registration ---
	cmdRegistry := &commands{
		Handlers: make(map[string]commandHandlerFunc),
	}
	cmdRegistry.register("reset", handlerReset)
	cmdRegistry.register("register", handlerRegister)
	cmdRegistry.register("login", handlerLogin)
	cmdRegistry.register("addfeed", handlerAddFeed)
	cmdRegistry.register("agg", handlerAgg)

	// --- 3. Argument Handling and Execution ---
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
