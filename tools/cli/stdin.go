package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// isStdinPiped checks if stdin is being piped to the program
func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// scanStdin reads input from stdin and returns a slice of non-empty lines
func scanStdin() ([]string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading from stdin: %w", err)
	}
	return lines, nil
}
