package main

import (
	"testing"
)

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{
			name:    "valid default format",
			input:   "default",
			want:    FormatDefault,
			wantErr: false,
		},
		{
			name:    "valid one-line format",
			input:   "one-line",
			want:    FormatOneLine,
			wantErr: false,
		},
		{
			name:    "valid json format",
			input:   "json",
			want:    FormatJSON,
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty format",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputFormat(t *testing.T) {
	tests := []struct {
		name string
		f    OutputFormat
		want string
	}{
		{
			name: "default format",
			f:    FormatDefault,
			want: "default",
		},
		{
			name: "one-line format",
			f:    FormatOneLine,
			want: "one-line",
		},
		{
			name: "json format",
			f:    FormatJSON,
			want: "json",
		},
		{
			name: "empty format",
			f:    "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.f); got != tt.want {
				t.Errorf("OutputFormat = %v, want %v", got, tt.want)
			}
		})
	}
}
