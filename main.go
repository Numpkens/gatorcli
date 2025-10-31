package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Numpkens/gatorcli/internal/config"
	"github.com/Numpkens/gatorcli/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type state struct {
	db  *database.Queries
	cfg *config.Config
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// 2. Open DB Connection
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()

	dbQueries := database.New(db)

	appState := &state{
		db:  dbQueries,
		cfg: cfg,
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . [command] [args]")
		os.Exit(1)
	}
	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "register":
		handleRegister(appState, args)
	case "login":
		handleLogin(appState, args)
	case "reset":
		handleReset(appState)
	case "users": // <-- ADDED
		handleUsers(appState) // <-- ADDED
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func handleRegister(s *state, args []string) {
	if len(args) == 0 {
		fmt.Println("Error: Please provide a name to register.")
		os.Exit(1)
	}
	userName := args[0]
	now := time.Now().UTC()

	_, err := s.db.GetUser(context.Background(), userName)
	if err == nil {
		fmt.Printf("Error: User '%s' already exists.\n", userName)
		os.Exit(1)
	}
	if err != sql.ErrNoRows {
		log.Fatalf("Error checking for existing user: %v", err)
	}

	params := database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      userName,
	}

	user, err := s.db.CreateUser(context.Background(), params)
	if err != nil {
		log.Fatalf("Error creating user: %v", err)
	}

	s.cfg.CurrentUserName = userName
	if err := s.cfg.Save(); err != nil {
		log.Fatalf("Error saving configuration: %v", err)
	}

	fmt.Printf("User '%s' registered and logged in.\n", userName)
	fmt.Printf("Debug: User created: %+v\n", user)
}

func handleLogin(s *state, args []string) {
	if len(args) == 0 {
		fmt.Println("Error: Please provide a name to login.")
		os.Exit(1)
	}
	userName := args[0]

	_, err := s.db.GetUser(context.Background(), userName)
	if err == sql.ErrNoRows {
		fmt.Printf("Error: User '%s' does not exist.\n", userName)
		os.Exit(1)
	}
	if err != nil {
		log.Fatalf("Error checking for user: %v", err)
	}

	s.cfg.CurrentUserName = userName
	if err := s.cfg.Save(); err != nil {
		log.Fatalf("Error saving configuration: %v", err)
	}

	fmt.Printf("Logged in as user '%s'.\n", userName)
}

func handleReset(s *state) {
	err := s.db.DeleteAllUsers(context.Background())
	if err != nil {
		fmt.Printf("Error: Failed to reset database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database successfully reset (all users deleted).")
}

// ⭐️ NEW FUNCTION ⭐️
func handleUsers(s *state) {
	// Call the generated GetUsers query
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		log.Fatalf("Error getting users: %v", err)
	}

	// Get the currently logged-in user name from the config
	currentUserName := s.cfg.CurrentUserName

	// Iterate and print in the specified format
	for _, user := range users {
		status := ""
		if user.Name == currentUserName {
			status = " (current)"
		}
		fmt.Printf("* %s%s\n", user.Name, status)
	}
}
