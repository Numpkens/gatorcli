module github.com/Numpkens/gatorcli

go 1.25.2

require (
	github.com/google/uuid v1.6.0
	github.com/mitchellh/go-homedir v1.1.0
	golang.org/x/net v0.46.0
	golang.org/x/text v0.30.0
)

// This directive forces the Go toolchain to look locally for your module,
// resolving the internal/feed import permanently.
replace github.com/Numpkens/gatorcli => ./
