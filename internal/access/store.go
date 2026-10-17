package access

import (
	"errors"
	"sync"
)

// ErrNoFile is returned by edits when Antennarr was started without -config,
// since there would be nowhere to keep the change.
var ErrNoFile = errors.New("start Antennarr with -config to edit settings")

// Store applies settings edits: validate, write the file, then apply to the
// running policy. If any step fails nothing changes, so the file and the
// running state never disagree.
type Store struct {
	path string
	pol  *Policy

	mu sync.Mutex // serializes read-modify-write cycles
}

func NewStore(path string, pol *Policy) *Store {
	return &Store{path: path, pol: pol}
}

// Editable reports whether edits can be saved.
func (s *Store) Editable() bool { return s.path != "" }

// Edit runs fn on a copy of the current settings and, if the result is valid,
// saves and applies it. It returns the settings now in force.
func (s *Store) Edit(fn func(*Settings) error) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return Settings{}, ErrNoFile
	}
	cur := s.pol.Settings()
	if err := fn(&cur); err != nil {
		return s.pol.Settings(), err
	}
	if err := cur.Validate(); err != nil {
		return s.pol.Settings(), &ValidationError{err}
	}
	if err := SaveSettings(s.path, cur); err != nil {
		return s.pol.Settings(), err
	}
	if err := s.pol.Update(cur); err != nil {
		return s.pol.Settings(), err
	}
	return cur, nil
}

// ValidationError marks an edit refused because the result would be invalid,
// as opposed to a failure to save it.
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }
