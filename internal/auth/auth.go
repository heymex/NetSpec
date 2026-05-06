package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "netspec_session"
const sessionDuration = 24 * time.Hour

// Manager handles password validation, session lifecycle, and bearer token checks.
type Manager struct {
	passwordHash []byte
	apiToken     string
	sessions     map[string]time.Time
	mu           sync.Mutex
}

// NewManager returns an auth manager. passwordHash must be a bcrypt hash string;
// empty means auth is disabled. apiToken is an optional plain bearer token.
func NewManager(passwordHash, apiToken string) *Manager {
	m := &Manager{
		sessions: make(map[string]time.Time),
		apiToken: apiToken,
	}
	if passwordHash != "" {
		m.passwordHash = []byte(passwordHash)
	}
	return m
}

// Enabled reports whether authentication is active.
func (m *Manager) Enabled() bool {
	return m.passwordHash != nil
}

// ValidatePassword returns true if password matches the stored bcrypt hash.
func (m *Manager) ValidatePassword(password string) bool {
	if !m.Enabled() {
		return false
	}
	return bcrypt.CompareHashAndPassword(m.passwordHash, []byte(password)) == nil
}

// CreateSession creates a new session and returns its ID.
func (m *Manager) CreateSession() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.URLEncoding.EncodeToString(b)
	m.mu.Lock()
	m.sessions[id] = time.Now().Add(sessionDuration)
	m.mu.Unlock()
	return id, nil
}

// ValidateSession returns true if the session ID is known and not expired.
func (m *Manager) ValidateSession(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp, ok := m.sessions[id]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(m.sessions, id)
		return false
	}
	return true
}

// DeleteSession removes a session.
func (m *Manager) DeleteSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// IsAuthenticated returns true if the request carries a valid session cookie
// or a matching bearer token.
func (m *Manager) IsAuthenticated(r *http.Request) bool {
	if m.apiToken != "" {
		if r.Header.Get("Authorization") == "Bearer "+m.apiToken {
			return true
		}
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return m.ValidateSession(c.Value)
}

// SessionCookie returns a cookie that stores the given session ID.
func (m *Manager) SessionCookie(id string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionDuration.Seconds()),
	}
}

// ClearCookie returns an expired cookie that clears the session on the client.
func (m *Manager) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
}

// HashPassword generates a bcrypt hash of password suitable for storage.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
