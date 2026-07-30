package operator

import (
	"sync"
	"time"
)

// Throttle slows repeated failed logins for one username.
//
// Keyed by username rather than source address, so the protection follows the
// account an attacker is actually trying to break into — a botnet rotating IPs
// against one account is still throttled. The converse is not covered: a
// distributed attempt spread thinly across many usernames needs a request rate
// limiter at the edge, which is a deployment concern rather than this package's.
//
// State is process-local, so a restart clears it. That is a real weakness of
// having no durable adapter, and it is why the lockout is a delay rather than
// the only line of defence.
type Throttle struct {
	mu       sync.Mutex
	attempts map[string]*attemptState

	// MaxAttempts failures are allowed before any delay applies.
	MaxAttempts int
	// BaseDelay is the delay after the first failure past MaxAttempts, doubling
	// with each subsequent failure up to MaxDelay.
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

type attemptState struct {
	failures    int
	lockedUntil time.Time
}

const (
	defaultMaxAttempts = 5
	defaultBaseDelay   = 2 * time.Second
	defaultMaxDelay    = 5 * time.Minute
)

func NewThrottle() *Throttle {
	return &Throttle{
		attempts:    make(map[string]*attemptState),
		MaxAttempts: defaultMaxAttempts,
		BaseDelay:   defaultBaseDelay,
		MaxDelay:    defaultMaxDelay,
	}
}

// Allowed reports whether username may attempt a login now. When it may not, the
// returned duration is how long the caller should tell the client to wait.
func (t *Throttle) Allowed(username string, now time.Time) (time.Duration, bool) {
	key := normalizeUsername(username)

	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.attempts[key]
	if !ok || state.lockedUntil.IsZero() || now.After(state.lockedUntil) {
		return 0, true
	}
	return state.lockedUntil.Sub(now), false
}

// RecordFailure counts a failed attempt and extends the lockout if warranted.
func (t *Throttle) RecordFailure(username string, now time.Time) {
	key := normalizeUsername(username)

	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.attempts[key]
	if !ok {
		state = &attemptState{}
		t.attempts[key] = state
	}
	state.failures++

	if state.failures <= t.MaxAttempts {
		return
	}

	delay := t.BaseDelay
	for range state.failures - t.MaxAttempts - 1 {
		delay *= 2
		if delay >= t.MaxDelay {
			delay = t.MaxDelay
			break
		}
	}
	state.lockedUntil = now.Add(delay)
}

// RecordSuccess clears the failure history for username.
func (t *Throttle) RecordSuccess(username string) {
	key := normalizeUsername(username)

	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, key)
}

// Failures reports the current consecutive failure count, for tests.
func (t *Throttle) Failures(username string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.attempts[normalizeUsername(username)]
	if !ok {
		return 0
	}
	return state.failures
}
