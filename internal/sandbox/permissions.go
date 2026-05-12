package sandbox

import (
	"path/filepath"
	"strings"
	"sync"
)

// SessionPermissionStore tracks session-scoped write path and command permissions.
type SessionPermissionStore struct {
	mu            sync.RWMutex
	pathGrants    map[string]map[string]bool // sessionKey → absPath → true
	commandGrants map[string]map[string]bool // sessionKey → commandName → true
}

// NewSessionPermissionStore creates a new SessionPermissionStore.
func NewSessionPermissionStore() *SessionPermissionStore {
	return &SessionPermissionStore{
		pathGrants:    make(map[string]map[string]bool),
		commandGrants: make(map[string]map[string]bool),
	}
}

// Grant adds a write permission for the given path in the given session.
func (s *SessionPermissionStore) Grant(sessionKey, absPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pathGrants[sessionKey] == nil {
		s.pathGrants[sessionKey] = make(map[string]bool)
	}
	s.pathGrants[sessionKey][absPath] = true
}

// IsGranted checks whether the given path (or a parent of it) is granted
// for the given session. Uses path-separator-aware prefix matching so that
// granting write to a directory also covers files inside it.
func (s *SessionPermissionStore) IsGranted(sessionKey, absPath string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := s.pathGrants[sessionKey]
	if paths == nil {
		return false
	}

	// Exact match
	if paths[absPath] {
		return true
	}

	// Parent path match: granted="/data/project" matches "/data/project/file.txt"
	sep := string(filepath.Separator)
	for granted := range paths {
		if strings.HasPrefix(absPath, granted+sep) {
			return true
		}
	}
	return false
}

// GrantCommand adds a command execution permission for the given session.
func (s *SessionPermissionStore) GrantCommand(sessionKey, commandName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandGrants[sessionKey] == nil {
		s.commandGrants[sessionKey] = make(map[string]bool)
	}
	s.commandGrants[sessionKey][commandName] = true
}

// IsCommandGranted checks whether the given command is granted for the session.
func (s *SessionPermissionStore) IsCommandGranted(sessionKey, commandName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	commands := s.commandGrants[sessionKey]
	if commands == nil {
		return false
	}
	return commands[commandName]
}

// RevokeAll removes all permissions (path and command) for a session.
func (s *SessionPermissionStore) RevokeAll(sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pathGrants, sessionKey)
	delete(s.commandGrants, sessionKey)
}
