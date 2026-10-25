package trae

import (
	"context"
	"errors"
	"time"
	"work2api/internal/statebackup"
	"work2api/internal/trae/auth"
	"work2api/internal/trae/upstream"
)

type credentialFlight struct {
	done      chan struct{}
	err       error
	refreshed bool
}

var errRefreshInterrupted = errors.New("credential refresh leader canceled")
var errRefreshPublished = errors.New("credential refreshed before leader canceled")
var errMaintenanceExpired = errors.New("Trae 登录已过期，请网页登录以取得刷新凭据")

type credentialRefreshError struct{ cause error }

func (e *credentialRefreshError) Error() string {
	return "Trae 凭据刷新失败，请稍后重试或重新授权"
}
func (e *credentialRefreshError) Unwrap() error { return e.cause }
func (e *credentialRefreshError) requiresLogin() bool {
	var remote *upstream.Error
	return errors.Is(e.cause, errMaintenanceExpired) || errors.As(e.cause, &remote) && remote.Status == 401
}

func (r *Runtime) refreshCredentials(ctx context.Context, a *auth.Auth, force bool) error {
	for {
		err := r.refreshCredentialsOnce(ctx, a, force)
		if errors.Is(err, errRefreshPublished) {
			force = false
			continue
		}
		if errors.Is(err, errRefreshInterrupted) {
			continue
		}
		return err
	}
}

func (r *Runtime) waitCredential(ctx context.Context, active *credentialFlight) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-active.done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(active.err, context.Canceled) || errors.Is(active.err, context.DeadlineExceeded) {
			if active.refreshed {
				return errRefreshPublished
			}
			return errRefreshInterrupted
		}
		return active.err
	}
}

// Network refresh is serialized per credential, not across the channel. The
// publication/write lock only guards the short, ownership-checked atomic save.
func (r *Runtime) refreshCredentialsOnce(ctx context.Context, a *auth.Auth, force bool) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := accountID(a.UID)
	r.mu.Lock()
	current := r.accounts[id]
	valid := current != nil && current.Identities[a.Variant] == a
	active := r.credentialFlights[a]
	dirty := r.credentialDirty[a]
	r.mu.Unlock()
	if !valid {
		return errors.New("Trae 账号授权已变更，请重试")
	}
	if active != nil {
		return r.waitCredential(ctx, active)
	}
	for !a.TryRLock() {
		r.mu.Lock()
		active = r.credentialFlights[a]
		r.mu.Unlock()
		if active != nil {
			return r.waitCredential(ctx, active)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
	refreshable := a.RefreshToken != ""
	expired := a.ExpiresAt > 0 && time.Now().Unix() >= a.ExpiresAt
	needed := refreshable && (a.NeedsRefreshLocked(10*time.Minute) || auth.IssuedTooLongLocked(a.AccessToken))
	a.RUnlock()
	if !refreshable {
		if expired {
			return &credentialRefreshError{cause: errMaintenanceExpired}
		}
		return ctx.Err()
	}
	if !force && !needed && !dirty {
		return ctx.Err()
	}
	r.mu.Lock()
	if active = r.credentialFlights[a]; active != nil {
		r.mu.Unlock()
		return r.waitCredential(ctx, active)
	}
	if r.credentialFlights == nil {
		r.credentialFlights = map[*auth.Auth]*credentialFlight{}
	}
	flight := &credentialFlight{done: make(chan struct{})}
	r.credentialFlights[a] = flight
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		flight.err = result
		current := r.accounts[id]
		if current == nil || current.Identities[a.Variant] != a {
			delete(r.credentialDirty, a)
		}
		delete(r.credentialFlights, a)
		close(flight.done)
		r.mu.Unlock()
	}()
	leave := statebackup.Enter()
	defer leave()
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	current = r.accounts[id]
	valid = current != nil && current.Identities[a.Variant] == a
	r.mu.Unlock()
	if !valid {
		return errors.New("Trae 账号授权已变更，请重试")
	}
	changed := false
	var err error
	if force {
		err = r.client(ctx).RefreshToken(a)
		changed = err == nil
	} else {
		changed, err = r.client(ctx).RefreshTokenIfNeeded(a, 10*time.Minute)
	}
	if changed {
		r.mu.Lock()
		if r.credentialDirty == nil {
			r.credentialDirty = map[*auth.Auth]bool{}
		}
		r.credentialDirty[a] = true
		r.mu.Unlock()
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &credentialRefreshError{cause: err}
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	current = r.accounts[id]
	valid = current != nil && current.Identities[a.Variant] == a
	dirty = r.credentialDirty[a]
	if !valid {
		delete(r.credentialDirty, a)
	}
	r.mu.Unlock()
	if !valid {
		return errors.New("Trae 账号授权已变更，请重试")
	}
	// A successful rotation must persist even if the initiating client canceled.
	// On disk failure keep a dirty marker so a fresh token still retries saving.
	if dirty {
		if err = a.SaveAtomic(); err != nil {
			return err
		}
		r.mu.Lock()
		delete(r.credentialDirty, a)
		r.mu.Unlock()
	}
	flight.refreshed = changed
	return ctx.Err()
}
