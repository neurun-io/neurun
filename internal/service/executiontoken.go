package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/repository"
)

// ErrUnknownExecutionToken is a token that is not live. A spent, expired or
// invented one reads identically, so nothing can be probed.
var ErrUnknownExecutionToken = errors.New("execution token is not valid")

// ExecutionIdentity is who a handler is, resolved rather than claimed.
type ExecutionIdentity struct {
	OrganizationID string `json:"organization_id"`
	AppID          string `json:"app_id"`
	ExecutionID    string `json:"execution_id"`
}

// ExecutionTokenService issues the credential a running handler calls back with.
//
// The handler is the tenant's own code, so anything it tells us about itself is
// a claim: an app id in its environment is a value it can change. A token is the
// one thing it holds that we minted, and everything it identifies lives on our
// side of the lookup.
type ExecutionTokenService struct {
	cache repository.Cache
	apps  *repository.AppRepository
	ttl   time.Duration
}

func NewExecutionTokenService(
	cache repository.Cache,
	apps *repository.AppRepository,
	ttl time.Duration,
) (*ExecutionTokenService, error) {
	if cache == nil || apps == nil {
		return nil, errors.New("execution token service requires a cache and apps")
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &ExecutionTokenService{cache: cache, apps: apps, ttl: ttl}, nil
}

// Mint issues a token for one execution, resolving the organization now so the
// handler never sends one and cannot name another.
func (service *ExecutionTokenService) Mint(
	ctx context.Context,
	executionID, appID string,
) (string, error) {
	organizationID, err := service.apps.OrganizationOf(ctx, appID)
	if err != nil {
		return "", fmt.Errorf("resolve execution organization: %w", err)
	}
	encoded, err := json.Marshal(ExecutionIdentity{
		OrganizationID: organizationID,
		AppID:          appID,
		ExecutionID:    executionID,
	})
	if err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	// Keyed by digest, so a dump of the cache hands nobody a working credential
	// — the same rule the session tokens follow.
	if err := service.cache.Set(
		ctx, executionTokenKey(token), encoded, service.ttl,
	); err != nil {
		return "", err
	}
	return token, nil
}

func (service *ExecutionTokenService) Resolve(
	ctx context.Context,
	token string,
) (ExecutionIdentity, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ExecutionIdentity{}, ErrUnknownExecutionToken
	}
	encoded, found, err := service.cache.Get(ctx, executionTokenKey(token))
	if err != nil {
		return ExecutionIdentity{}, err
	}
	if !found {
		return ExecutionIdentity{}, ErrUnknownExecutionToken
	}
	var identity ExecutionIdentity
	if err := json.Unmarshal(encoded, &identity); err != nil {
		return ExecutionIdentity{}, fmt.Errorf("decode execution identity: %w", err)
	}
	return identity, nil
}

// Revoke spends the token, so a leaked one cannot outlive the run that held it.
func (service *ExecutionTokenService) Revoke(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return service.cache.Delete(ctx, executionTokenKey(token))
}

func executionTokenKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return "execution-token:" + hex.EncodeToString(digest[:])
}
