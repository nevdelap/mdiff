package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/util"
)

// InteractiveRunner lets an embedding application own terminal and signal
// handoff while retaining Micro's interactive shell entry point.
var InteractiveRunner func(string, bool, bool) (string, error)

// ExecCommand executes a command using exec
// It returns any output/errors
func ExecCommand(name string, arg ...string) (string, error) {
	var err error
	cmd := exec.Command(name, arg...)
	outputBytes := &bytes.Buffer{}
	cmd.Stdout = outputBytes
	cmd.Stderr = outputBytes
	err = cmd.Start()
	if err != nil {
		return "", err
	}
	err = cmd.Wait() // wait for command to finish
	outstring := outputBytes.String()
	return outstring, err
}

// RunCommand executes a shell command and returns the output/error
func RunCommand(input string) (string, error) {
	args, err := shellquote.Split(input)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", errors.New("No arguments")
	}
	inputCmd := args[0]

	return ExecCommand(inputCmd, args[1:]...)
}

// RunBackgroundShell runs a shell command in the background
// It returns a function which will run the command and returns a string
// message result
func RunBackgroundShell(input string) (func() string, error) {
	args, err := shellquote.Split(input)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("No arguments")
	}
	inputCmd := args[0]
	return func() string {
		output, err := RunCommand(input)

		str := output
		if err != nil {
			str = fmt.Sprint(inputCmd, " exited with error: ", err, ": ", output)
		}
		return str
	}, nil
}

// RunInteractiveShell runs a shellcommand interactively
func RunInteractiveShell(input string, wait bool, getOutput bool) (string, error) {
	if InteractiveRunner != nil {
		return InteractiveRunner(input, wait, getOutput)
	}
	command, err := PrepareInteractiveCommand(input, wait, getOutput)
	if err != nil {
		return "", err
	}
	var screenWasNil bool
	c := make(chan os.Signal, 1)
	return command.execute(interactiveHooks{
		before: func() {
			screenWasNil = screen.TempFini()
			// Keep Micro's standalone Ctrl+C handling through the Enter prompt.
			signal.Reset(os.Interrupt)
			signal.Notify(c, os.Interrupt)
		},
		run:     runCommand,
		message: screen.TermMessage,
		after: func() {
			_ = screen.TempStart(screenWasNil)
			signal.Notify(util.Sigterm, os.Interrupt)
			signal.Stop(c)
		},
	})
}

// InteractiveCommand is a validated Micro shell command ready for execution.
// Preparing it before terminal handoff leaves invalid input in the editor.
type InteractiveCommand struct {
	args            []string
	wait, getOutput bool
}

// PrepareInteractiveCommand parses the command once for both Micro's standalone
// route and applications that own terminal handoff.
func PrepareInteractiveCommand(input string, wait, getOutput bool) (*InteractiveCommand, error) {
	args, err := shellquote.Split(input)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("No arguments")
	}
	return &InteractiveCommand{args: args, wait: wait, getOutput: getOutput}, nil
}

// RunWithReleasedTerminal executes a prepared command while the embedding
// application owns the released terminal and process signals.
func (command *InteractiveCommand) RunWithReleasedTerminal() (string, error) {
	// mdiff has released its screen, so use Micro's terminal prompt for the
	// command and restore the application's status handler before returning.
	messageHandler := screen.MessageHandler
	screen.SetMessageHandler(nil)
	defer screen.SetMessageHandler(messageHandler)
	return command.execute(interactiveHooks{
		run:     runCommand,
		message: screen.TermMessage,
	})
}

type interactiveHooks struct {
	before  func()
	run     func(*exec.Cmd) (bool, error)
	message func(...any)
	after   func()
}

func (command *InteractiveCommand) execute(hooks interactiveHooks) (string, error) {
	if hooks.before != nil {
		hooks.before()
	}

	// Set up everything for the command
	outputBytes := &bytes.Buffer{}
	cmd := exec.Command(command.args[0], command.args[1:]...)
	cmd.Stdin = os.Stdin
	if command.getOutput {
		cmd.Stdout = io.MultiWriter(os.Stdout, outputBytes)
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr

	started, err := hooks.run(cmd)
	if started {
		if command.wait {
			hooks.message("")
		}
	} else {
		hooks.message(err)
	}

	output := outputBytes.String()
	if hooks.after != nil {
		hooks.after()
	}

	return output, err
}

func runCommand(cmd *exec.Cmd) (bool, error) {
	if err := cmd.Start(); err != nil {
		return false, err
	}
	return true, cmd.Wait()
}

// UserCommand runs the shell command
// The openTerm argument specifies whether a terminal should be opened (for viewing output
// or interacting with stdin)
// func UserCommand(input string, openTerm bool, waitToFinish bool) string {
// 	if !openTerm {
// 		RunBackgroundShell(input)
// 		return ""
// 	} else {
// 		output, _ := RunInteractiveShell(input, waitToFinish, false)
// 		return output
// 	}
// }
