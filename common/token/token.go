package token

import (
	"crypto/subtle"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Manager handles token validation and rotation
type Manager struct {
	mu     sync.RWMutex
	tokens map[string][]string // name -> tokens
}

// NewManager creates a new token manager
func NewManager() *Manager {
	return &Manager{
		tokens: make(map[string][]string),
	}
}

// LoadFromEnv loads tokens from environment variables with the given prefix
// Supports both single token: TOKEN_PREFIX_name=token
// And multiple tokens: TOKEN_PREFIX_name=[token1,token2]
func (m *Manager) LoadFromEnv(prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key := pair[0]
		value := pair[1]

		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// Extract name from key (e.g., TOKEN_SOURCE_postgres-prod-01 -> postgres-prod-01)
		name := strings.TrimPrefix(key, prefix)
		if name == "" {
			continue
		}

		// Parse token value (supports single or array format)
		tokens := parseTokenValue(value)
		if len(tokens) > 0 {
			m.tokens[name] = tokens
		}
	}

	return nil
}

// parseTokenValue parses token value which can be:
// - Single token: "abc123"
// - Multiple tokens: "[token1,token2,token3]"
func parseTokenValue(value string) []string {
	value = strings.TrimSpace(value)

	// Check if it's an array format
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		// Remove brackets and split by comma
		inner := strings.TrimPrefix(value, "[")
		inner = strings.TrimSuffix(inner, "]")

		tokens := []string{}
		for _, token := range strings.Split(inner, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				tokens = append(tokens, token)
			}
		}
		return tokens
	}

	// Single token
	if value != "" {
		return []string{value}
	}

	return nil
}

// Validate checks if the provided token is valid for the given name
func (m *Manager) Validate(name, token string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens, exists := m.tokens[name]
	if !exists {
		return false
	}

	for _, validToken := range tokens {
		if subtle.ConstantTimeCompare([]byte(validToken), []byte(token)) == 1 {
			return true
		}
	}

	return false
}

// GetNames returns all registered names
func (m *Manager) GetNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.tokens))
	for name := range m.tokens {
		names = append(names, name)
	}
	return names
}

// Reload reloads tokens from environment (for SIGHUP handling)
// Loads new tokens first, then swaps atomically under a single lock
// to avoid a window where all tokens are rejected.
func (m *Manager) Reload(prefix string) error {
	newTokens := make(map[string][]string)

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) != 2 {
			continue
		}

		key := pair[0]
		value := pair[1]

		if !strings.HasPrefix(key, prefix) {
			continue
		}

		name := strings.TrimPrefix(key, prefix)
		if name == "" {
			continue
		}

		tokens := parseTokenValue(value)
		if len(tokens) > 0 {
			newTokens[name] = tokens
		}
	}

	m.mu.Lock()
	m.tokens = newTokens
	m.mu.Unlock()

	return nil
}

// String returns a string representation (for debugging, without actual tokens)
func (m *Manager) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.tokens))
	for name, tokens := range m.tokens {
		names = append(names, fmt.Sprintf("%s(%d tokens)", name, len(tokens)))
	}
	return strings.Join(names, ", ")
}
