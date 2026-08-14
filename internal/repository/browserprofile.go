package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

const browserProfileColumns = `id, organization_id, name, identity,
	cookies, local_storage, session_storage, created_at, updated_at`

type BrowserProfileRepository struct {
	pool *pgxpool.Pool
}

func NewBrowserProfileRepository(pool *pgxpool.Pool) (*BrowserProfileRepository, error) {
	if pool == nil {
		return nil, errors.New("browser profile repository requires a database pool")
	}
	return &BrowserProfileRepository{pool: pool}, nil
}

func scanBrowserProfile(row pgx.CollectableRow) (browser.Profile, error) {
	var record browser.Profile
	var identity, cookies, localStorage, sessionStorage []byte
	if err := row.Scan(
		&record.ID, &record.OrganizationID, &record.Name,
		&identity, &cookies, &localStorage, &sessionStorage,
		&record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		return browser.Profile{}, err
	}
	if err := json.Unmarshal(identity, &record.Identity); err != nil {
		return browser.Profile{}, fmt.Errorf("decode identity: %w", err)
	}
	if err := json.Unmarshal(cookies, &record.Cookies); err != nil {
		return browser.Profile{}, fmt.Errorf("decode cookies: %w", err)
	}
	if err := json.Unmarshal(localStorage, &record.LocalStorage); err != nil {
		return browser.Profile{}, fmt.Errorf("decode local storage: %w", err)
	}
	if err := json.Unmarshal(sessionStorage, &record.SessionStorage); err != nil {
		return browser.Profile{}, fmt.Errorf("decode session storage: %w", err)
	}
	if err := record.Validate(); err != nil {
		return browser.Profile{}, fmt.Errorf("invalid persisted browser profile: %w", err)
	}
	return record, nil
}

// browserProfileDocuments encodes the four jsonb columns in column order.
func browserProfileDocuments(record browser.Profile) ([][]byte, error) {
	// A nil slice or map marshals to null, which the column CHECKs reject.
	if record.Cookies == nil {
		record.Cookies = []browser.Cookie{}
	}
	if record.LocalStorage == nil {
		record.LocalStorage = browser.Storage{}
	}
	if record.SessionStorage == nil {
		record.SessionStorage = browser.Storage{}
	}

	documents := make([][]byte, 0, 4)
	for _, value := range []any{
		record.Identity, record.Cookies, record.LocalStorage, record.SessionStorage,
	} {
		document, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode browser profile: %w", err)
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func (repository *BrowserProfileRepository) Create(
	ctx context.Context,
	record browser.Profile,
) (browser.Profile, error) {
	if err := record.Validate(); err != nil {
		return browser.Profile{}, err
	}
	documents, err := browserProfileDocuments(record)
	if err != nil {
		return browser.Profile{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`INSERT INTO browser_profiles
		 (id, organization_id, name, identity,
		  cookies, local_storage, session_storage, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+browserProfileColumns,
		record.ID, record.OrganizationID, record.Name,
		documents[0], documents[1], documents[2], documents[3],
		record.CreatedAt, record.UpdatedAt,
	)
	if err != nil {
		return browser.Profile{}, fmt.Errorf(
			"%w: create browser profile: %v", browser.ErrConflict, err,
		)
	}
	created, err := pgx.CollectExactlyOneRow(rows, scanBrowserProfile)
	if err != nil {
		return browser.Profile{}, fmt.Errorf(
			"%w: create browser profile: %v", browser.ErrConflict, err,
		)
	}
	return created, nil
}

// GetByID addresses a profile within one organization. A profile belonging to
// another organization reads as absent, never as forbidden.
func (repository *BrowserProfileRepository) GetByID(
	ctx context.Context,
	organizationID string,
	profileID string,
) (browser.Profile, error) {
	if err := browser.ValidateIdentifier("browser_profile_id", profileID); err != nil {
		return browser.Profile{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+browserProfileColumns+`
		 FROM browser_profiles WHERE id = $1 AND organization_id = $2`,
		profileID, organizationID,
	)
	if err != nil {
		return browser.Profile{}, fmt.Errorf("read browser profile: %w", err)
	}
	record, err := pgx.CollectExactlyOneRow(rows, scanBrowserProfile)
	if errors.Is(err, pgx.ErrNoRows) {
		return browser.Profile{}, fmt.Errorf("%w: %s", browser.ErrNotFound, profileID)
	}
	if err != nil {
		return browser.Profile{}, fmt.Errorf("read browser profile: %w", err)
	}
	return record, nil
}

// List returns profiles newest first.
func (repository *BrowserProfileRepository) List(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]browser.Profile, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+browserProfileColumns+`
		 FROM browser_profiles WHERE organization_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2`,
		organizationID, postgresLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list browser profiles: %w", err)
	}
	records, err := pgx.CollectRows(rows, scanBrowserProfile)
	if err != nil {
		return nil, fmt.Errorf("list browser profiles: %w", err)
	}
	return records, nil
}

func (repository *BrowserProfileRepository) Update(
	ctx context.Context,
	record browser.Profile,
) (browser.Profile, error) {
	if err := record.Validate(); err != nil {
		return browser.Profile{}, err
	}
	documents, err := browserProfileDocuments(record)
	if err != nil {
		return browser.Profile{}, err
	}
	rows, err := repository.pool.Query(
		ctx,
		`UPDATE browser_profiles
		 SET name = $3, identity = $4, cookies = $5,
		     local_storage = $6, session_storage = $7, updated_at = $8
		 WHERE id = $1 AND organization_id = $2
		 RETURNING `+browserProfileColumns,
		record.ID, record.OrganizationID, record.Name,
		documents[0], documents[1], documents[2], documents[3],
		record.UpdatedAt,
	)
	if err != nil {
		return browser.Profile{}, fmt.Errorf(
			"%w: update browser profile: %v", browser.ErrConflict, err,
		)
	}
	updated, err := pgx.CollectExactlyOneRow(rows, scanBrowserProfile)
	if errors.Is(err, pgx.ErrNoRows) {
		return browser.Profile{}, fmt.Errorf("%w: %s", browser.ErrNotFound, record.ID)
	}
	if err != nil {
		return browser.Profile{}, fmt.Errorf(
			"%w: update browser profile: %v", browser.ErrConflict, err,
		)
	}
	return updated, nil
}

func (repository *BrowserProfileRepository) Delete(
	ctx context.Context,
	organizationID string,
	profileID string,
) error {
	if err := browser.ValidateIdentifier("browser_profile_id", profileID); err != nil {
		return err
	}
	tag, err := repository.pool.Exec(
		ctx,
		`DELETE FROM browser_profiles WHERE id = $1 AND organization_id = $2`,
		profileID, organizationID,
	)
	if err != nil {
		return fmt.Errorf("delete browser profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", browser.ErrNotFound, profileID)
	}
	return nil
}
