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

	_ "github.com/lib/pq"

	"github.com/google/uuid"

	"github.com/Numpkens/gatorcli/internal/config"
	"github.com/Numpkens/gatorcli/internal/database"
	"github.com/Numpkens/gatorcli/internal/feed"
)

type state struct {
	Config *config.Config
	DB     *database.Queries
	DBConn *sql.DB
}

type command struct {
	Name string
	Args []string
}

type commandHandlerFunc func(s *state, cmd command) error

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

	// Truncate the parent table ('users') and use CASCADE to automatically clear the dependent 'feeds' table.
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

	// 1. Insert the new user into the database
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

// FIX: This function now looks up the user by name to get the correct UUID
func handlerLogin(s *state, cmd command) error {
	if len(cmd.Args) == 0 {
		return errors.New("login command requires a single argument: <username>")
	}

	username := cmd.Args[0]

	// 1. Look up the user by name to get their actual UUID
	user, err := s.DB.GetUser(context.Background(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user '%s' not found. Please register first.", username)
		}
		return fmt.Errorf("failed to look up user: %w", err)
	}

	// 2. Set the actual user ID in the config
	if err := s.Config.SetUser(user.ID.String()); err != nil {
		return fmt.Errorf("failed to set current user: %w", err)
	}

	fmt.Printf("Successfully set current user to: %s (ID: %s)\n", username, user.ID.String())

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

// ENHANCED: Now automatically creates a feed follow record.
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

	// Auto-create feed follow for the user who added the feed
	_, err = s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		// FIX: Use uuid.NullUUID
		ID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
		UserID:    uuid.NullUUID{UUID: userID, Valid: true},
		FeedID:    uuid.NullUUID{UUID: newFeed.ID, Valid: true},
	})

	if err != nil {
		log.Printf("Warning: Could not auto-create feed follow: %v", err)
	}

	fmt.Printf("Successfully added new feed and started following it:\n")
	fmt.Printf("  ID:        %s\n", newFeed.ID)
	fmt.Printf("  Name:      %s\n", newFeed.Name)
	fmt.Printf("  URL:       %s\n", newFeed.Url)
	fmt.Printf("  User ID:   %s\n", newFeed.UserID)
	fmt.Printf("  Created At: %s\n", newFeed.CreatedAt)

	return nil
}

func handlerListFeeds(s *state, cmd command) error {
	ctx := context.Background()

	feedsWithUsers, err := s.DB.GetFeedsWithUserName(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch feeds: %w", err)
	}

	if len(feedsWithUsers) == 0 {
		fmt.Println("No feeds found in the database.")
		return nil
	}

	fmt.Printf("Found %d feeds:\n", len(feedsWithUsers))
	fmt.Println("--------------------------------------------------------------------------------")
	for _, feed := range feedsWithUsers {
		fmt.Printf("Feed Name:  %s\n", feed.Name)
		fmt.Printf("URL:        %s\n", feed.Url)
		fmt.Printf("Created By: %s\n", feed.UserName)
		fmt.Println("--------------------------------------------------------------------------------")
	}

	return nil
}

// NEW COMMAND: Handles 'gator follow <url>'
func handlerFollow(s *state, cmd command) error {
	if len(cmd.Args) != 1 {
		return errors.New("follow command requires a single argument: <url>")
	}
	feedURL := cmd.Args[0]

	if s.Config.UserID == "" {
		return errors.New("user is not logged in. Please run 'gator login <username>' first")
	}

	// 1. Get the current user ID
	userID, err := uuid.Parse(s.Config.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID in config: %w", err)
	}

	// 2. Look up the feed by URL
	feed, err := s.DB.GetFeedByUrl(context.Background(), feedURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("feed with URL '%s' not found. Please add the feed first using 'gator addfeed'", feedURL)
		}
		return fmt.Errorf("failed to look up feed: %w", err)
	}

	// 3. Create the feed follow record
	now := time.Now().UTC()
	follow, err := s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		// FIX: Use uuid.NullUUID
		ID:        uuid.NullUUID{UUID: uuid.New(), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
		UserID:    uuid.NullUUID{UUID: userID, Valid: true},
		FeedID:    uuid.NullUUID{UUID: feed.ID, Valid: true},
	})

	if err != nil {
		// Handle unique constraint violation specifically
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errors.New("you are already following this feed")
		}
		return fmt.Errorf("failed to create feed follow: %w", err)
	}

	// 4. Print confirmation
	fmt.Printf("User %s is now following feed %s.\n", follow.UserName, follow.FeedName)

	return nil
}

// NEW COMMAND: Handles 'gator following'
func handlerFollowing(s *state, cmd command) error {
	if s.Config.UserID == "" {
		return errors.New("user is not logged in. Please run 'gator login <username>' first")
	}

	userID, err := uuid.Parse(s.Config.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID in config: %w", err)
	}

	// FIX: Convert the non-nullable uuid.UUID to nullable uuid.NullUUID as required by sqlc generated code.
	follows, err := s.DB.GetFeedFollowsForUser(context.Background(), uuid.NullUUID{UUID: userID, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to fetch feed follows: %w", err)
	}

	if len(follows) == 0 {
		fmt.Println("You are not currently following any feeds.")
		return nil
	}

	fmt.Printf("You are following %d feeds:\n", len(follows))
	for _, follow := range follows {
		fmt.Printf("  - %s\n", follow.FeedName)
	}

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
		// Ensure these credentials are correct for your local PostgreSQL setup
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
		DBConn: dbConn,
	}

	// --- 2. Command Registration ---
	cmdRegistry := &commands{
		Handlers: make(map[string]commandHandlerFunc),
	}
	cmdRegistry.register("reset", handlerReset)
	cmdRegistry.register("register", handlerRegister)
	cmdRegistry.register("login", handlerLogin)
	cmdRegistry.register("addfeed", handlerAddFeed)
	cmdRegistry.register("feeds", handlerListFeeds)
	cmdRegistry.register("follow", handlerFollow)       // NEW Command
	cmdRegistry.register("following", handlerFollowing) // NEW Command
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
