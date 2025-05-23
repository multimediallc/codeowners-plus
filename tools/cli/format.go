package main

import (
	"fmt"
	"slices"
	"strings"
)

type OutputFormat string

const (
	FormatDefault OutputFormat = "default"
	FormatOneLine OutputFormat = "one-line"
	FormatJSON    OutputFormat = "json"
)

var allowedFormats = []string{string(FormatDefault), string(FormatOneLine), string(FormatJSON)}

func validateFormat(format string) (OutputFormat, error) {
	if !slices.Contains(allowedFormats, format) {
		return "", fmt.Errorf("invalid format %s. Must be one of %s", format, strings.Join(allowedFormats, ", "))
	}
	return OutputFormat(format), nil
}
