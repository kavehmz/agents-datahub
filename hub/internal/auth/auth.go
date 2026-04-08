package auth

import (
	"fmt"
	"sync"

	"github.com/regentmarkets/agents-datahub/common/token"
	"github.com/regentmarkets/agents-datahub/hub/internal/config"
)

// Authorizer handles authentication and authorization
type Authorizer struct {
	sourceTokens  *token.Manager
	exposerTokens *token.Manager
	permissions   map[string][]PermissionRule // exposer name -> permissions
	mu            sync.RWMutex
}

// PermissionRule defines what an exposer can access
type PermissionRule struct {
	Label      string
	Operations []string // ["*"] means all operations
}

// NewAuthorizer creates a new authorizer
func NewAuthorizer(cfg *config.Config) (*Authorizer, error) {
	a := &Authorizer{
		sourceTokens:  token.NewManager(),
		exposerTokens: token.NewManager(),
		permissions:   make(map[string][]PermissionRule),
	}

	// Load source tokens from environment
	if err := a.sourceTokens.LoadFromEnv("TOKEN_SOURCE_"); err != nil {
		return nil, fmt.Errorf("failed to load source tokens: %w", err)
	}

	// Load exposer tokens from environment
	if err := a.exposerTokens.LoadFromEnv("TOKEN_EXPOSER_"); err != nil {
		return nil, fmt.Errorf("failed to load exposer tokens: %w", err)
	}

	// Load permissions from config
	for _, exposer := range cfg.Exposers {
		rules := make([]PermissionRule, len(exposer.Permissions))
		for i, perm := range exposer.Permissions {
			rules[i] = PermissionRule{
				Label:      perm.Label,
				Operations: perm.Operations,
			}
		}
		a.permissions[exposer.Name] = rules
	}

	return a, nil
}

// AuthenticateSource validates a source token
func (a *Authorizer) AuthenticateSource(name, authToken string) bool {
	return a.sourceTokens.Validate(name, authToken)
}

// AuthenticateExposer validates an exposer token
func (a *Authorizer) AuthenticateExposer(name, authToken string) bool {
	return a.exposerTokens.Validate(name, authToken)
}

// AuthorizeOperation checks if an exposer can perform an operation on a label
func (a *Authorizer) AuthorizeOperation(exposerName, label, operation string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rules, exists := a.permissions[exposerName]
	if !exists {
		return false
	}

	for _, rule := range rules {
		// Check if label matches (support wildcards)
		if rule.Label != "*" && rule.Label != label {
			continue
		}

		// Check if operation matches (support wildcards)
		if containsOrWildcard(rule.Operations, operation) {
			return true
		}
	}

	return false
}

// containsOrWildcard checks if a slice contains the value or "*"
func containsOrWildcard(slice []string, value string) bool {
	for _, item := range slice {
		if item == "*" || item == value {
			return true
		}
	}
	return false
}

// ReloadTokens reloads tokens from environment
func (a *Authorizer) ReloadTokens() error {
	if err := a.sourceTokens.Reload("TOKEN_SOURCE_"); err != nil {
		return fmt.Errorf("failed to reload source tokens: %w", err)
	}
	if err := a.exposerTokens.Reload("TOKEN_EXPOSER_"); err != nil {
		return fmt.Errorf("failed to reload exposer tokens: %w", err)
	}
	return nil
}

// GetSourceNames returns all registered source names
func (a *Authorizer) GetSourceNames() []string {
	return a.sourceTokens.GetNames()
}

// GetExposerNames returns all registered exposer names
func (a *Authorizer) GetExposerNames() []string {
	return a.exposerTokens.GetNames()
}

// GetPermissions returns permissions for an exposer
func (a *Authorizer) GetPermissions(exposerName string) []PermissionRule {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rules, exists := a.permissions[exposerName]
	if !exists {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]PermissionRule, len(rules))
	copy(result, rules)
	return result
}
