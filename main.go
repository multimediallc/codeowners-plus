package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/multimediallc/codeowners-plus/internal/app"
)

// Flags holds the command line flags
type Flags struct {
	Token   *string
	RepoDir *string
	PR      *int
	Repo    *string
	Verbose *bool
	Quiet   *bool
}

var (
	flags = &Flags{
		Token:   flag.String("token", getEnv("INPUT_GITHUB-TOKEN", ""), "GitHub authentication token"),
		RepoDir: flag.String("dir", getEnv("GITHUB_WORKSPACE", "/"), "Path to local Git repo"),
		PR:      flag.Int("pr", ignoreError(strconv.Atoi(getEnv("INPUT_PR", ""))), "Pull Request number"),
		Repo:    flag.String("repo", getEnv("INPUT_REPOSITORY", ""), "GitHub repo name"),
		Verbose: flag.Bool("v", ignoreError(strconv.ParseBool(getEnv("INPUT_VERBOSE", "0"))), "Verbose output"),
		Quiet:   flag.Bool("quiet", ignoreError(strconv.ParseBool(getEnv("INPUT_QUIET", "0"))), "Disable PR comments and review requests"),
	}
	WarningBuffer = bytes.NewBuffer([]byte{})
	InfoBuffer    = bytes.NewBuffer([]byte{})
)

// initFlags initializes and parses command line flags
func initFlags(flags *Flags) error {
	// Only parse flags if we're not testing
	if !testing.Testing() {
		flag.Parse()
	}

	// Validate required flags
	badFlags := make([]string, 0, 4)
	if *flags.Token == "" {
		badFlags = append(badFlags, "token")
	}
	if *flags.PR == 0 {
		badFlags = append(badFlags, "pr")
	}
	if *flags.Repo == "" {
		badFlags = append(badFlags, "repo")
	}
	if len(badFlags) > 0 {
		return fmt.Errorf("required flags or environment variables not set: %s", badFlags)
	}

	return nil
}

// Helper functions
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func ignoreError[V any, E error](res V, _ E) V {
	return res
}

func outputAndExit(w io.Writer, shouldFail bool, message string) {
	_, err := WarningBuffer.WriteTo(w)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing warning buffer: %v\n", err)
	}
	if *flags.Verbose {
		_, err := InfoBuffer.WriteTo(w)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing info buffer: %v\n", err)
		}
	}

	_, _ = fmt.Fprint(w, message)
	if testing.Testing() {
		return
	}
	if shouldFail {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

// writeGITHUBOUTPUT writes the OutputData to the GITHUB_OUTPUT file in the correct format
func writeGITHUBOUTPUT(outputData *app.OutputData) error {
	githubOutput := os.Getenv("GITHUB_OUTPUT")
	if githubOutput == "" {
		return nil // No GITHUB_OUTPUT environment variable set
	}

	// Marshal the entire OutputData to JSON
	jsonData, err := json.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("error marshaling JSON output: %w", err)
	}

	// Use GitHub Actions delimiter approach for robust handling of special characters
	output := fmt.Sprintf("data<<EOF\n%s\nEOF\n", string(jsonData))
	err = os.WriteFile(githubOutput, []byte(output), 0644)
	if err != nil {
		return fmt.Errorf("error writing to GITHUB_OUTPUT: %w", err)
	}

	return nil
}

func main() {
	err := initFlags(flags)
	if err != nil {
		outputAndExit(os.Stderr, true, fmt.Sprintln(err))
	}

	cfg := app.Config{
		Token:         *flags.Token,
		RepoDir:       *flags.RepoDir,
		PR:            *flags.PR,
		Repo:          *flags.Repo,
		Verbose:       *flags.Verbose,
		Quiet:         *flags.Quiet,
		InfoBuffer:    InfoBuffer,
		WarningBuffer: WarningBuffer,
	}

	app, err := app.New(cfg)
	if err != nil {
		outputAndExit(os.Stderr, true, fmt.Sprintf("Failed to initialize app: %v\n", err))
	}

	outputData, err := app.Run()
	if err != nil {
		outputAndExit(os.Stderr, true, fmt.Sprintln(err))
	}

	// Write JSON output to GITHUB_OUTPUT if the environment variable is set
	if err := writeGITHUBOUTPUT(outputData); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	var w io.Writer
	if outputData.Success {
		w = os.Stdout
	} else {
		w = os.Stderr
	}
	shouldFail := !outputData.Success && app.Conf.Enforcement.FailCheck
	outputAndExit(w, shouldFail, outputData.Message)
}
