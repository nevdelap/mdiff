package buffer

import (
	"sync"

	"github.com/nevdelap/mdiff/internal/micro/pkg/highlight"
)

// highlightSnapshot is detached text and metadata used by syntax workers.
// Highlighter goroutines never traverse a live Micro buffer.
type highlightSnapshot struct {
	mu      sync.Mutex
	lines   [][]byte
	states  []highlight.State
	matches []highlight.LineMatch
}

func (s *highlightSnapshot) Lock()   { s.mu.Lock() }
func (s *highlightSnapshot) Unlock() { s.mu.Unlock() }
func (s *highlightSnapshot) LinesNum() int {
	return len(s.lines)
}
func (s *highlightSnapshot) LineBytes(line int) []byte { return s.lines[line] }
func (s *highlightSnapshot) State(line int) highlight.State {
	return s.states[line]
}
func (s *highlightSnapshot) SetState(line int, state highlight.State) {
	s.states[line] = state
}
func (s *highlightSnapshot) SetMatch(line int, match highlight.LineMatch) {
	s.matches[line] = match
}

func (s *highlightSnapshot) Match(line int) highlight.LineMatch {
	return s.matches[line]
}
