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

// --- MIDDLEWARE ---

func middlewareLoggedIn(handler func(s *state, cmd command, user database.User) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		if s.Config.UserID == "" {
			return errors.New("user is not logged in. Please run 'gator login <username>' first")
		}

		userID, err := uuid.Parse(s.Config.UserID)
		if err != nil {
			return fmt.Errorf("invalid user ID in config: %w", err)
		}

		user, err := s.DB.GetUserByID(context.Background(), userID)
		if err != nil {
			return fmt.Errorf("failed to fetch user from database: %w", err)
		}

		return handler(s, cmd, user)
	}
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

	if err := s.Config.SetUser(userID.String()); err != nil {
		return fmt.Errorf("failed to register and set current user: %w", err)
	}

	fmt.Printf("User %s registered successfully (ID: %s).\n", username, userID.String())
	return nil
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.Args) == 0 {
		return errors.New("login command requires a single argument: <username>")
	}

	username := cmd.Args[0]
	user, err := s.DB.GetUser(context.Background(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user '%s' not found. Please register first.", username)
		}
		return fmt.Errorf("failed to look up user: %w", err)
	}

	if err := s.Config.SetUser(user.ID.String()); err != nil {
		return fmt.Errorf("failed to set current user: %w", err)
	}

	fmt.Printf("Successfully set current user to: %s (ID: %s)\n", username, user.ID.String())
	return nil
}

// New aggregation function
func scrapeFeeds(s *state) {
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Get the next feed to fetch from the DB.
	feed, err := s.DB.GetNextFeedToFetch(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// This is normal if there are no feeds in the DB
			return
		}
		log.Printf("Error getting next feed to fetch: %v", err)
		return
	}

	fmt.Printf(">> Fetching feed: %s from %s\n", feed.Name, feed.Url)

	// 2. Mark it as fetched.
	err = s.DB.MarkFeedFetched(ctx, database.MarkFeedFetchedParams{
		ID:            feed.ID,
		LastFetchedAt: now,
		UpdatedAt:     now,
	})
	if err != nil {
		log.Printf("Error marking feed %s as fetched: %v", feed.Name, err)
	}

	// 3. Fetch the feed using the URL.
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// NOTE: The feed.FetchFeed function is assumed to be available from the internal/feed package
	rssFeed, err := feed.FetchFeed(fetchCtx, feed.Url)
	if err != nil {
		log.Printf("Error fetching RSS feed %s (%s): %v", feed.Name, feed.Url, err)
		return
	}

	// 4. Iterate over items and print titles.
	fmt.Printf("   Successfully fetched %d posts from %s\n", len(rssFeed.Channel.Item), feed.Name)
	for _, item := range rssFeed.Channel.Item {
		fmt.Printf("   - %s\n", item.Title)
	}
	fmt.Println("<< Done with feed.")
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.Args) != 1 {
		return errors.New("agg command requires a single argument: <time_between_reqs> (e.g., 30s, 1m)")
	}
	timeBetweenReqsStr := cmd.Args[0]

	timeBetweenRequests, err := time.ParseDuration(timeBetweenReqsStr)
	if err != nil {
		return fmt.Errorf("failed to parse duration string '%s'. Example formats: 1s, 30m, 1h: %w", timeBetweenReqsStr, err)
	}

	fmt.Printf("Collecting feeds every %s...\n", timeBetweenRequests)
	fmt.Println("Press Ctrl+C to stop the process.")

	ticker := time.NewTicker(timeBetweenRequests)
	defer ticker.Stop()

	// Run immediately
	scrapeFeeds(s)

	// Loop forever, running on every tick
	for ; ; <-ticker.C {
		scrapeFeeds(s)
	}
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.Args) != 2 {
		return errors.New("addfeed command requires two arguments: <name> <url>")
	}
	feedName := cmd.Args[0]
	feedURL := cmd.Args[1]

	userID := user.ID
	now := time.Now().UTC()
	feedID := uuid.New()
	feedFollowID := uuid.New()

	newFeed, err := s.DB.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        feedID,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      feedName,
		Url:       feedURL,
		UserID:    userID,
	})
	if err != nil {
		return fmt.Errorf("failed to create feed in database: %w", err)
	}

	_, err = s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        feedFollowID,
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		FeedID:    feedID,
	})
	if err != nil {
		log.Printf("Warning: Could not auto-create feed follow: %v", err)
	}

	follow, err := s.DB.GetFeedFollowForUserAndFeed(context.Background(), database.GetFeedFollowForUserAndFeedParams{
		UserID: userID,
		FeedID: feedID,
	})
	if err != nil {
		log.Printf("Warning: Failed to fetch feed follow join data for printing: %v", err)
	}

	fmt.Printf("Successfully added new feed and started following it:\n")
	fmt.Printf("  ID:        %s\n", newFeed.ID)
	fmt.Printf("  Name:      %s\n", newFeed.Name)
	fmt.Printf("  URL:       %s\n", newFeed.Url)
	fmt.Printf("  User Name: %s\n", follow.UserName)
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

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("follow command requires a single argument: <url>")
	}
	feedURL := cmd.Args[0]
	userID := user.ID

	feed, err := s.DB.GetFeedByUrl(context.Background(), feedURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("feed with URL '%s' not found. Please add the feed first using 'gator addfeed'", feedURL)
		}
		return fmt.Errorf("failed to look up feed: %w", err)
	}

	now := time.Now().UTC()
	feedFollowID := uuid.New()
	_, err = s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        feedFollowID,
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		FeedID:    feed.ID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errors.New("you are already following this feed")
		}
		return fmt.Errorf("failed to create feed follow: %w", err)
	}

	follow, err := s.DB.GetFeedFollowForUserAndFeed(context.Background(), database.GetFeedFollowForUserAndFeedParams{
		UserID: userID,
		FeedID: feed.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to fetch follow confirmation data: %w", err)
	}

	fmt.Printf("User %s is now following feed %s.\n", follow.UserName, follow.FeedName)
	return nil
}

func handlerFollowing(s *state, cmd command, user database.User) error {
	userID := user.ID
	follows, err := s.DB.GetFeedFollowsForUser(context.Background(), userID)
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

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("unfollow command requires a single argument: <url>")
	}
	feedURL := cmd.Args[0]
	userID := user.ID

	feed, err := s.DB.GetFeedByUrl(context.Background(), feedURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("feed with URL '%s' not found. You can only unfollow existing feeds.", feedURL)
		}
		return fmt.Errorf("failed to look up feed: %w", err)
	}

	err = s.DB.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: userID,
		FeedID: feed.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to unfollow feed: %w", err)
	}

	fmt.Printf("Successfully unfollowed feed: %s\n", feed.Name)
	return nil
}

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Error reading initial config: %v", err)
	}

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
		DBConn: dbConn,
	}

	cmdRegistry := &commands{Handlers: make(map[string]commandHandlerFunc)}
	cmdRegistry.register("reset", handlerReset)
	cmdRegistry.register("register", handlerRegister)
	cmdRegistry.register("login", handlerLogin)
	cmdRegistry.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmdRegistry.register("feeds", handlerListFeeds)
	cmdRegistry.register("follow", middlewareLoggedIn(handlerFollow))
	cmdRegistry.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	cmdRegistry.register("following", middlewareLoggedIn(handlerFollowing))
	cmdRegistry.register("agg", handlerAgg)

	args := os.Args
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: Not enough arguments provided. Command name is required.")
		os.Exit(1)
	}

	commandName := strings.ToLower(args[1])
	commandArgs := args[2:]

	currentCommand := command{Name: commandName, Args: commandArgs}
	if err := cmdRegistry.run(appState, currentCommand); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command '%s': %v\n", commandName, err)
		os.Exit(1)
	}
}
