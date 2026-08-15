package service

import (
	"context"
	"errors"
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
)

// BrowserService owns browser profiles.
//
// It does not open browsers. The browser server runs on loopback beside the
// SDK, which reads a profile, drives the session itself, and writes the state
// back through SaveState. Nothing here holds a connection to it — the control
// plane could not reach a loopback address anyway.
type BrowserService struct {
	profiles *database.BrowserProfileRepository
	now      func() time.Time
	newID    func(string) (string, error)
}

func NewBrowserService(
	profiles *database.BrowserProfileRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*BrowserService, error) {
	if profiles == nil {
		return nil, errors.New("browser service requires its repository")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = ids.New
	}
	return &BrowserService{profiles: profiles, now: now, newID: newID}, nil
}

func (service *BrowserService) Create(
	ctx context.Context,
	organizationID string,
	name string,
	identity *browser.Identity,
) (browser.Profile, error) {
	id, err := service.newID("bp")
	if err != nil {
		return browser.Profile{}, err
	}
	record, err := browser.New(id, organizationID, name, identity, service.now())
	if err != nil {
		return browser.Profile{}, err
	}
	return service.profiles.Create(ctx, record)
}

func (service *BrowserService) Get(
	ctx context.Context,
	organizationID, profileID string,
) (browser.Profile, error) {
	return service.profiles.GetByID(ctx, organizationID, profileID)
}

func (service *BrowserService) List(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]browser.Profile, error) {
	return service.profiles.List(ctx, organizationID, limit)
}

func (service *BrowserService) Rename(
	ctx context.Context,
	organizationID, profileID, name string,
) (browser.Profile, error) {
	record, err := service.profiles.GetByID(ctx, organizationID, profileID)
	if err != nil {
		return browser.Profile{}, err
	}
	if err := record.Rename(name, service.now()); err != nil {
		return browser.Profile{}, err
	}
	return service.profiles.Update(ctx, record)
}

// SetIdentity replaces the presentation half and leaves the profile's cookies
// and storage where they are.
func (service *BrowserService) SetIdentity(
	ctx context.Context,
	organizationID, profileID string,
	identity browser.Identity,
) (browser.Profile, error) {
	record, err := service.profiles.GetByID(ctx, organizationID, profileID)
	if err != nil {
		return browser.Profile{}, err
	}
	if err := record.SetIdentity(identity, service.now()); err != nil {
		return browser.Profile{}, err
	}
	return service.profiles.Update(ctx, record)
}

// SaveState replaces the state half with what a closing session captured.
//
// It overwrites rather than merges: the browser hands back its whole cookie jar
// and storage, so a cookie missing from it was deleted, and merging would
// resurrect logins the site had already ended.
func (service *BrowserService) SaveState(
	ctx context.Context,
	organizationID, profileID string,
	cookies []browser.Cookie,
	local, session browser.Storage,
) (browser.Profile, error) {
	record, err := service.profiles.GetByID(ctx, organizationID, profileID)
	if err != nil {
		return browser.Profile{}, err
	}
	if err := record.Capture(cookies, local, session, service.now()); err != nil {
		return browser.Profile{}, err
	}
	return service.profiles.Update(ctx, record)
}

func (service *BrowserService) Delete(
	ctx context.Context,
	organizationID, profileID string,
) error {
	return service.profiles.Delete(ctx, organizationID, profileID)
}
