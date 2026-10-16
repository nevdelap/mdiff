package prototype

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/screen"
)

func TestInvalidInteractiveShellInputKeepsEditorScreen(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"empty command", ""},
		{"malformed quoting", "printf 'unterminated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, sim := newEditingTestApp(t, 60, 10)
			defer app.Close()
			handoffs := 0
			app.handoff = func(func() error) error {
				handoffs++
				t.Fatal("invalid shell input released the terminal")
				return nil
			}
			output, err := app.runInteractiveShell(tc.input, true, true)
			if err == nil || output != "" || handoffs != 0 {
				t.Fatalf("invalid shell input: output=%q err=%v handoffs=%d", output, err, handoffs)
			}
			if app.terminal != sim || screen.Screen != sim || !strings.Contains(action.InfoBar.Msg, err.Error()) {
				t.Fatal("invalid shell input changed the screen or lost its error status")
			}
			app.handle(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone, ""))
			if string(app.left.buf.Bytes()) != "x" {
				t.Fatal("editor did not accept an edit after invalid shell input")
			}
		})
	}
}

func TestInteractiveShellAppRouteCommandSemantics(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	app.handoff = func(work func() error) error {
		active := screen.Screen
		screen.Screen = nil
		defer func() { screen.Screen = active }()
		return work()
	}

	input, err := os.CreateTemp(t.TempDir(), "shell-input-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	if _, err := input.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	oldStdin, oldStdout := os.Stdin, os.Stdout
	defer func() { os.Stdin, os.Stdout = oldStdin, oldStdout }()
	os.Stdin = input

	for _, tc := range []struct {
		name, command                       string
		wait, capture                       bool
		wantOutput, wantTerminal, wantError string
		wantPrompt                          bool
	}{
		{"captured output", "printf CAPTURED-OUTPUT", false, true, "CAPTURED-OUTPUT", "CAPTURED-OUTPUT", "", false},
		{"uncaptured output", "printf VISIBLE-OUTPUT", false, false, "", "VISIBLE-OUTPUT", "", false},
		{"nonzero without wait", "false", false, false, "", "", "exit status 1", false},
		{"nonzero with wait", "false", true, false, "", "Press enter to continue", "exit status 1", true},
		{"start failure waits without wait flag", "mdiff-nonexistent-shell-command", false, false, "", "Press enter to continue", "executable file not found", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := input.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = writer
			output, commandErr := app.runInteractiveShell(tc.command, tc.wait, tc.capture)
			_ = writer.Close()
			os.Stdout = oldStdout
			terminal, readErr := io.ReadAll(reader)
			_ = reader.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if output != tc.wantOutput || !strings.Contains(string(terminal), tc.wantTerminal) {
				t.Fatalf("output=%q terminal=%q", output, terminal)
			}
			if strings.Contains(string(terminal), "Press enter to continue") != tc.wantPrompt {
				t.Fatalf("prompt mismatch: %q", terminal)
			}
			if tc.wantError == "" {
				if commandErr != nil {
					t.Fatal(commandErr)
				}
			} else if commandErr == nil || !strings.Contains(commandErr.Error(), tc.wantError) ||
				!strings.Contains(action.InfoBar.Msg, tc.wantError) {
				t.Fatalf("error=%v status=%q", commandErr, action.InfoBar.Msg)
			}
			if tc.name == "nonzero without wait" && strings.Contains(string(terminal), "exit status") {
				t.Fatalf("wait=false printed exit error to terminal: %q", terminal)
			}
			if screen.MessageHandler == nil {
				t.Fatal("shell route did not restore the editor status handler")
			}
		})
	}
}
