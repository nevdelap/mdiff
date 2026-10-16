package prototype

import (
	"fmt"
	"sync/atomic"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/screen"
)

// terminalReader is the only goroutine that polls a particular tcell screen.
// It is stopped and joined before that screen is released.
type terminalReader struct {
	terminal tcell.Screen
	stop     chan struct{}
	done     chan struct{}
}

func startTerminalReader(terminal tcell.Screen, events chan<- tcell.Event) *terminalReader {
	r := &terminalReader{terminal: terminal, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		for {
			event := terminal.PollEvent()
			if event == nil {
				return
			}
			select {
			case <-r.stop:
				return
			default:
			}
			select {
			case <-r.stop:
				return
			case events <- event:
			}
		}
	}()
	return r
}

func (r *terminalReader) close() {
	close(r.stop)
	select {
	case <-r.done:
		return
	default:
	}
	_ = r.terminal.PostEvent(tcell.NewEventKey(tcell.KeyNUL, 0, tcell.ModNone, ""))
	<-r.done
}

type terminalSession struct {
	app       *App
	events    chan tcell.Event
	reader    *terminalReader
	inCommand atomic.Bool
	restart   func(bool) error
}

func newTerminalSession(app *App, events chan tcell.Event) *terminalSession {
	s := &terminalSession{app: app, events: events, restart: screen.TempStart}
	s.reader = startTerminalReader(app.terminal, events)
	return s
}

func (s *terminalSession) stopReader() {
	if s.reader != nil {
		s.reader.close()
		s.reader = nil
	}
}

// handoff runs work with the terminal in normal mode, then attaches a fresh
// reader and resumes drawing. Only a screen restart failure is fatal.
func (s *terminalSession) handoff(work func() error) error {
	if s.app.terminal == nil || screen.Screen != s.app.terminal {
		return fmt.Errorf("terminal screen is unavailable for handoff")
	}
	s.inCommand.Store(true)
	s.stopReader()
	for len(s.events) > 0 {
		<-s.events
	}
	wasNil := screen.TempFini()
	s.app.terminal = nil
	workErr := work()
	if err := s.restart(wasNil); err != nil {
		s.inCommand.Store(false)
		return fmt.Errorf("restart terminal after handoff: %w", err)
	}
	if screen.Screen == nil {
		s.inCommand.Store(false)
		return fmt.Errorf("restart terminal after handoff: no screen was initialized")
	}
	s.app.terminal = screen.Screen
	s.app.resize(s.app.terminal.Size())
	s.drainDiff()
	s.reader = startTerminalReader(s.app.terminal, s.events)
	s.inCommand.Store(false)
	if workErr != nil && action.InfoBar != nil {
		action.InfoBar.Error(workErr.Error())
	}
	return nil
}

func (s *terminalSession) drainDiff() {
	select {
	case <-s.app.coordinator.TimerC():
		left, right := s.app.snapshots()
		s.app.coordinator.DebounceElapsedOwned(diffSnapshot{left: left, right: right, versions: s.app.versions})
	default:
	}
	for {
		select {
		case result := <-s.app.coordinator.Results():
			s.app.installDiff(result)
		default:
			return
		}
	}
}
