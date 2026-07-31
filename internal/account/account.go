package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/neurun-io/neurun/internal/auth"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/operator"
)

var (
	ErrInvalid  = errors.New("invalid account request")
	ErrNotFound = errors.New("account resource not found")
	ErrConflict = errors.New("account resource conflict")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

type User struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateUserRequest struct {
	ProjectID, Username, DisplayName, Role, Password string
}
type UpdateUserRequest struct {
	DisplayName *string
	Role        *string
	Disabled    *bool
}

type Key struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	UserID    string     `json:"user_id,omitempty"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type CreatedKey struct {
	Key
	Secret string `json:"secret"`
}
type CreateKeyRequest struct {
	ProjectID, UserID, Name string
	Scopes                  []string
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type OperatorStore struct {
	accounts *Store
	sessions *operator.MemoryStore
}

func NewOperatorStore(accounts *Store) (*OperatorStore, error) {
	if accounts == nil {
		return nil, errors.New("account store is required")
	}
	sessions, err := operator.NewMemoryStore()
	if err != nil {
		return nil, err
	}
	return &OperatorStore{accounts: accounts, sessions: sessions}, nil
}

func (store *OperatorStore) AccountByUsername(
	ctx context.Context, username string,
) (operator.Account, error) {
	var account operator.Account
	var role string
	err := store.accounts.db.QueryRowContext(ctx, `SELECT id,username,role,
		project_id,password_hash,disabled,created_at FROM users
		WHERE username=$1`, strings.ToLower(strings.TrimSpace(username))).
		Scan(&account.ID, &account.Username, &role, &account.ProjectID,
			&account.PasswordHash, &account.Disabled, &account.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return operator.Account{}, operator.ErrAccountNotFound
	}
	if err != nil {
		return operator.Account{}, err
	}
	account.Role = operator.Role(role)
	return account, nil
}

func (store *OperatorStore) CreateSession(
	ctx context.Context, account operator.Account, token string, expiresAt time.Time,
) (operator.Session, error) {
	return store.sessions.CreateSession(ctx, account, token, expiresAt)
}

func (store *OperatorStore) SessionByToken(
	ctx context.Context, token string, now time.Time,
) (operator.Session, error) {
	session, err := store.sessions.SessionByToken(ctx, token, now)
	if err != nil {
		return operator.Session{}, err
	}
	var role string
	var disabled bool
	err = store.accounts.db.QueryRowContext(ctx,
		`SELECT role,disabled FROM users WHERE id=$1 AND project_id=$2`,
		session.AccountID, session.ProjectID).Scan(&role, &disabled)
	if err != nil || disabled {
		_ = store.sessions.DeleteSession(ctx, token)
		return operator.Session{}, operator.ErrSessionNotFound
	}
	session.Role = operator.Role(role)
	return session, nil
}

func (store *OperatorStore) DeleteSession(ctx context.Context, token string) error {
	return store.sessions.DeleteSession(ctx, token)
}

func (store *OperatorStore) DeleteExpiredSessions(
	ctx context.Context, now time.Time,
) (int, error) {
	return store.sessions.DeleteExpiredSessions(ctx, now)
}

func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("account database is required")
	}
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) CreateUser(ctx context.Context, r CreateUserRequest) (User, error) {
	r.Username, r.DisplayName, r.Role = strings.ToLower(strings.TrimSpace(r.Username)),
		strings.TrimSpace(r.DisplayName), strings.TrimSpace(r.Role)
	if r.ProjectID == "" || !usernamePattern.MatchString(r.Username) || !validDisplayName(r.DisplayName) ||
		!validRole(r.Role) || len(r.Username) > 64 || len(r.DisplayName) > 128 {
		return User{}, fmt.Errorf("%w: user fields are incomplete", ErrInvalid)
	}
	passwordHash, err := operator.HashPassword(r.Password)
	if err != nil {
		return User{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	id, err := ids.New("usr")
	if err != nil {
		return User{}, err
	}
	now := s.now()
	user := User{ID: id, ProjectID: r.ProjectID, Username: r.Username,
		DisplayName: r.DisplayName, Role: r.Role, CreatedAt: now, UpdatedAt: now}
	_, err = s.db.ExecContext(ctx, `INSERT INTO users
		(id,project_id,username,display_name,role,password_hash,disabled,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,false,$7,$7)`,
		id, r.ProjectID, r.Username, r.DisplayName, r.Role, passwordHash, now)
	if err != nil {
		return User{}, classifyWriteError("create user", err)
	}
	return user, nil
}

func (s *Store) GetUser(ctx context.Context, projectID, id string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT id,project_id,username,
		display_name,role,disabled,created_at,updated_at FROM users
		WHERE project_id=$1 AND id=$2`, projectID, id))
}

func (s *Store) ListUsers(ctx context.Context, projectID string, limit int) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,project_id,username,
		display_name,role,disabled,created_at,updated_at FROM users
		WHERE project_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, user)
	}
	return result, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, projectID, id string, r UpdateUserRequest) (User, error) {
	user, err := s.GetUser(ctx, projectID, id)
	if err != nil {
		return User{}, err
	}
	if r.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*r.DisplayName)
	}
	if r.Role != nil {
		user.Role = strings.TrimSpace(*r.Role)
	}
	if r.Disabled != nil {
		user.Disabled = *r.Disabled
	}
	if !validDisplayName(user.DisplayName) || !validRole(user.Role) {
		return User{}, ErrInvalid
	}
	user.UpdatedAt = s.now()
	_, err = s.db.ExecContext(ctx, `UPDATE users SET display_name=$3,role=$4,
		disabled=$5,updated_at=$6 WHERE project_id=$1 AND id=$2`,
		projectID, id, user.DisplayName, user.Role, user.Disabled, user.UpdatedAt)
	return user, err
}

func (s *Store) CreateKey(ctx context.Context, r CreateKeyRequest) (CreatedKey, error) {
	if strings.TrimSpace(r.ProjectID) == "" || strings.TrimSpace(r.Name) == "" ||
		len(r.Name) > 128 || len(r.Scopes) > 32 {
		return CreatedKey{}, ErrInvalid
	}
	for _, scope := range r.Scopes {
		if !validScope(scope) {
			return CreatedKey{}, fmt.Errorf("%w: invalid scope", ErrInvalid)
		}
	}
	r.Scopes = normalizeScopes(r.Scopes)
	if r.UserID != "" {
		var exists bool
		err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM users WHERE id=$1 AND project_id=$2 AND disabled=false
		)`, r.UserID, r.ProjectID).Scan(&exists)
		if err != nil {
			return CreatedKey{}, err
		}
		if !exists {
			return CreatedKey{}, fmt.Errorf("%w: key owner was not found", ErrInvalid)
		}
	}
	id, err := ids.New("key")
	if err != nil {
		return CreatedKey{}, err
	}
	prefixRaw, secretRaw := make([]byte, 6), make([]byte, 32)
	if _, err = rand.Read(prefixRaw); err != nil {
		return CreatedKey{}, err
	}
	if _, err = rand.Read(secretRaw); err != nil {
		return CreatedKey{}, err
	}
	prefix := "neu_live_" + hex.EncodeToString(prefixRaw)
	secret := prefix + "." + hex.EncodeToString(secretRaw)
	digest := sha256.Sum256([]byte(secret))
	now := s.now()
	key := Key{ID: id, ProjectID: r.ProjectID, UserID: r.UserID,
		Name: strings.TrimSpace(r.Name), Prefix: prefix, Scopes: r.Scopes, CreatedAt: now}
	var owner any
	if r.UserID != "" {
		owner = r.UserID
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO api_keys
		(id,project_id,user_id,name,key_prefix,key_hash,scopes,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		id, r.ProjectID, owner, key.Name, prefix, digest[:], r.Scopes, now)
	if err != nil {
		return CreatedKey{}, classifyWriteError("create key", err)
	}
	return CreatedKey{Key: key, Secret: secret}, nil
}

func (s *Store) ListKeys(ctx context.Context, projectID string, limit int) ([]Key, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,project_id,COALESCE(user_id,''),
		name,key_prefix,to_jsonb(scopes)::text,created_at,revoked_at FROM api_keys WHERE project_id=$1
		ORDER BY created_at DESC,id DESC LIMIT $2`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Key, 0)
	for rows.Next() {
		var key Key
		if err := rows.Scan(&key.ID, &key.ProjectID, &key.UserID, &key.Name,
			&key.Prefix, (*scopeList)(&key.Scopes), &key.CreatedAt, &key.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

func (s *Store) RevokeKey(ctx context.Context, projectID, id string) (Key, error) {
	now := s.now()
	var key Key
	err := s.db.QueryRowContext(ctx, `UPDATE api_keys SET revoked_at=COALESCE(revoked_at,$3)
		WHERE project_id=$1 AND id=$2
		RETURNING id,project_id,COALESCE(user_id,''),name,key_prefix,to_jsonb(scopes)::text,created_at,revoked_at`,
		projectID, id, now).Scan(&key.ID, &key.ProjectID, &key.UserID, &key.Name,
		&key.Prefix, (*scopeList)(&key.Scopes), &key.CreatedAt, &key.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Key{}, ErrNotFound
	}
	if err != nil {
		return Key{}, err
	}
	return key, nil
}

// scopeList reads a text[] column that its query renders as JSON.
//
// The pgx stdlib driver hands database/sql the bare Postgres array literal, and
// database/sql cannot assign that string to a *[]string. Selecting the column
// through to_jsonb makes the wire form unambiguous — including for the empty
// array and for values that would otherwise need array-literal quoting — so
// decoding is a plain json.Unmarshal.
type scopeList []string

func (list *scopeList) Scan(src any) error {
	var raw []byte
	switch value := src.(type) {
	case nil:
		*list = nil
		return nil
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return fmt.Errorf("account: cannot scan %T into scopes", src)
	}
	var scopes []string
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return fmt.Errorf("account: decode scopes: %w", err)
	}
	*list = scopes
	return nil
}

type scanner interface{ Scan(...any) error }

func scanUser(row scanner) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.ProjectID, &user.Username, &user.DisplayName,
		&user.Role, &user.Disabled, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return user, err
}

func validRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

func validDisplayName(name string) bool {
	if name == "" || len(name) > 128 || !utf8.ValidString(name) {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validScope(scope string) bool {
	switch scope {
	case "*", "users:read", "users:write", "api_keys:read", "api_keys:write",
		"projects:read", "projects:write", "apps:read", "apps:write",
		"deployments:read", "deployments:write",
		"builds:read", "executions:read", "executions:write":
		return true
	default:
		return false
	}
}

func normalizeScopes(scopes []string) []string {
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if _, exists := seen[scope]; !exists {
			seen[scope] = struct{}{}
			normalized = append(normalized, scope)
		}
	}
	sort.Strings(normalized)
	return normalized
}

func classifyWriteError(operation string, err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) {
		switch postgres.Code {
		case "23505":
			return fmt.Errorf("%w: %s conflicts with an existing resource", ErrConflict, operation)
		case "23503", "23514":
			return fmt.Errorf("%w: %s violates a resource constraint", ErrInvalid, operation)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *Store) EnsureConfiguredKey(
	ctx context.Context, projectID, raw string, scopes []string,
) error {
	prefix, _, ok := strings.Cut(strings.TrimSpace(raw), ".")
	if !ok {
		return ErrInvalid
	}
	digest := sha256.Sum256([]byte(raw))
	id, err := ids.New("key")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO api_keys
		(id,project_id,name,key_prefix,key_hash,scopes,created_at)
		VALUES($1,$2,'Configured server key',$3,$4,$5,$6)
		ON CONFLICT(key_prefix) DO UPDATE SET
		project_id=EXCLUDED.project_id,key_hash=EXCLUDED.key_hash,
		scopes=EXCLUDED.scopes,revoked_at=NULL`,
		id, projectID, prefix, digest[:], scopes, s.now())
	return err
}

func (s *Store) AuthenticateContext(ctx context.Context, raw string) (auth.Principal, bool) {
	prefix, _, ok := strings.Cut(strings.TrimSpace(raw), ".")
	if !ok {
		return auth.Principal{}, false
	}
	var id, projectID string
	var scopes scopeList
	var stored []byte
	err := s.db.QueryRowContext(ctx, `SELECT id,project_id,to_jsonb(scopes)::text,key_hash
		FROM api_keys WHERE key_prefix=$1 AND revoked_at IS NULL`, prefix).
		Scan(&id, &projectID, &scopes, &stored)
	digest := sha256.Sum256([]byte(raw))
	if err != nil || len(stored) != sha256.Size ||
		subtle.ConstantTimeCompare(stored, digest[:]) != 1 {
		return auth.Principal{}, false
	}
	return auth.Principal{
		Kind: auth.KindAPIKey, KeyID: id, ProjectID: projectID, Scopes: scopes,
	}, true
}

func (s *Store) EnsureConfiguredUser(ctx context.Context, configured operator.Account) error {
	id := configured.ID
	if id == "" {
		var err error
		id, err = ids.New("usr")
		if err != nil {
			return err
		}
	}
	now := s.now()
	displayName := configured.Username
	_, err := s.db.ExecContext(ctx, `INSERT INTO users
		(id,project_id,username,display_name,role,password_hash,disabled,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT(username) DO UPDATE SET
		project_id=EXCLUDED.project_id,display_name=EXCLUDED.display_name,role=EXCLUDED.role,
		password_hash=EXCLUDED.password_hash,disabled=EXCLUDED.disabled,
		updated_at=EXCLUDED.updated_at`,
		id, configured.ProjectID, configured.Username, displayName, string(configured.Role),
		configured.PasswordHash, configured.Disabled, now)
	return err
}
