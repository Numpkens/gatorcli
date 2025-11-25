# 🐊 Gator CLI: RSS Feed Reader and Aggregator

Gator is a command-line interface tool for managing and aggregating RSS and Atom feeds. It allows you to register a user, follow feeds, and run a continuous background process to fetch the latest posts.

## Prerequisites

To run Gator, you need the following installed on your system:

1.  **Go (v1.20 or newer):** Gator is built in Go and requires the Go toolchain for installation and initial setup.
2.  **PostgreSQL:** Gator uses a PostgreSQL database for persistence. You must have a running Postgres instance available.

## Installation

Gator is designed to be installed as a standalone command-line executable.

1.  **Clone the repository:**
    ```bash
    git clone [YOUR_GITHUB_REPO_URL]
    cd gatorcli
    ```

2.  **Install the CLI:**
    Use `go install` to build the program and place the resulting binary (`gator`) in your Go bin path, making it accessible system-wide.
    ```bash
    go install
    ```
    *Note: If you are still developing, you can use `go run . <command>` for testing, but the production command is simply `gator <command>`.*

## Setup and Configuration

### 1. Database Setup

You must configure the connection string to your PostgreSQL database using the `DATABASE_URL` environment variable.

If you don't set `DATABASE_URL`, Gator will default to the following connection string, which assumes a local Postgres instance with default credentials:

```text
host=localhost port=5432 user=postgres password=postgres dbname=gatorcli sslmode=disable

### 2. Running Migrations

Before using the CLI, you need to set up the database schema.
Bash

# Run the reset command to set up the latest schema (or run migration tools if you have them)
gator reset

### 3. User Registration

Gator requires a registered user to track feed follows. The user ID is stored in a configuration file (~/.gatorcli.json) so the CLI knows who you are in subsequent runs.
Bash

gator register <your_username>

Usage and Commands

Once set up, you can interact with Gator using the following commands:
Command	Description	Example
register	Registers a new user and sets them as the current user.	gator register alice
login	Sets an existing user as the current user.	gator login alice
feeds	Lists all feeds known to the system.	gator feeds
addfeed	Adds a new feed and automatically follows it. (Requires login)	gator addfeed "Hacker News" "https://hnrss.org/newest"
follow	Starts following an existing feed URL. (Requires login)	gator follow "https://techcrunch.com/feed/"
unfollow	Stops following a feed URL. (Requires login)	gator unfollow "https://hnrss.org/newest"
following	Lists all feeds the current user is following. (Requires login)	gator following
agg	(Aggregator Loop) Runs the background feed fetching process.	gator agg 30s

The Aggregation Loop (agg) 

The agg command is designed to be run continuously in a separate terminal session.
Bash

gator agg <time_between_reqs>

    <time_between_reqs> is a Go duration string (e.g., 1s, 30m, 1h).

    The command will fetch the least-recently fetched feed, print the post titles, and then wait for the specified duration before repeating.

    Stop the process by pressing Ctrl+C.

Contributing and Development

Gator is open source! Feel free to fork the repository, make changes, and submit pull requests.

To Do:

    Persist fetched posts to the database.

    Implement a read or posts command to display saved posts.

    Add concurrency to the agg command to fetch multiple feeds in parallel.


