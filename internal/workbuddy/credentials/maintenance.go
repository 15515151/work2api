package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"work2api/internal/core/provider"
)

func maintenanceIdentity(s *Session) string {
	raw, _ := json.Marshal([]any{s.Auth["accessToken"], s.Auth["refreshToken"], s.Auth["domain"]})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) BindMaintenance(g *provider.MaintenanceGuard, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authGuard = g
	m.authKey = key
}

// AuthorizationInvalid excludes only proven credential failures, never a
// directory/billing permission failure or a temporary network error.
func (m *Manager) AuthorizationInvalid() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Healthy selection must not stat every credential file for every candidate.
	s := m.cached
	if s == nil {
		var err error
		s, err = m.session()
		if err != nil {
			return false
		}
	}
	invalid := func(s *Session) bool {
		return encryptedToken(s.Auth) || m.authGuard != nil && m.authGuard.State(m.authKey, maintenanceIdentity(s)).NeedsLogin
	}
	if !invalid(s) {
		return false
	}
	// A blocked identity checks for externally replaced credentials so desktop
	// reauthorization can restore it without restarting or changing enablement.
	if current, err := m.session(); err == nil {
		return invalid(current)
	}
	return true
}
