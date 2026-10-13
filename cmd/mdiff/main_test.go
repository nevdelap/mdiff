package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelpAndVersionExitSuccessfully(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "Usage: mdiff"},
		{name: "version", args: []string{"--version"}, want: "mdiff (wrapping micro) "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("run() exit = %d, want 0; stderr=%q", code, stderr.String())
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, test.want) {
				t.Fatalf("output %q does not contain %q", output, test.want)
			}
		})
	}
}

func TestCLIRejectsInvalidInputsBeforeStartingTerminal(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--no-such-flag"}, want: "flag provided but not defined"},
		{name: "debounce below range", args: []string{"--diff-debounce", "39"}, want: "between 40 and 180"},
		{name: "debounce above range", args: []string{"--diff-debounce", "181"}, want: "between 40 and 180"},
		{name: "too many paths", args: []string{"left", "right", "extra"}, want: "at most two file paths"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != 2 {
				t.Fatalf("run() exit = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), test.want)
			}
			if strings.Contains(stdout.String()+stderr.String(), "\x1b[") {
				t.Fatalf("invalid invocation emitted terminal control sequences: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestCLIRejectsMissingConfigDirectoryBeforeStartingTerminal(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-config")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--config-dir", missing}, &stdout, &stderr); code != 1 {
		t.Fatalf("run() exit = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "must already exist") {
		t.Fatalf("stderr %q does not explain the missing config directory", stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "\x1b[") {
		t.Fatalf("invalid invocation emitted terminal control sequences: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
