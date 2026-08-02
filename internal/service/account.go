package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/account"
	"github.com/neurun-io/neurun/internal/domain/auth"
	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository"
)

type AccountService struct {
	users *repository.UserRepository
	keys  *repository.APIKeyRepository
	now   func() time.Time
	newID func(string) (string, error)
}

func NewAccountService(
	users *repository.UserRepository,
	keys *repository.APIKeyRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*AccountService, error) {
	if users == nil || keys == nil {
		return nil, errors.New("account service requires its repositories")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = ids.New
	}
	return &AccountService{users: users, keys: keys, now: now, newID: newID}, nil
}

func (service *AccountService) CreateUser(
	ctx context.Context,
	request dto.CreateUserRequest,
) (account.User, error) {
	passwordHash, err := operator.HashPassword(request.Password)
	if err != nil {
		return account.User{}, fmt.Errorf("%w: %v", account.ErrInvalid, err)
	}
	id, err := service.newID("usr")
	if err != nil {
		return account.User{}, err
	}
	record, err := account.NewUser(
		id, request.Username, request.DisplayName, request.Role, service.now(),
	)
	if err != nil {
		return account.User{}, err
	}
	if err := service.users.Create(ctx, record, passwordHash); err != nil {
		return account.User{}, err
	}
	return record, nil
}

func (service *AccountService) GetUser(
	ctx context.Context,
	userID string,
) (account.User, error) {
	return service.users.GetByID(ctx, userID)
}

func (service *AccountService) ListUsers(
	ctx context.Context,
	limit int,
) ([]account.User, error) {
	return service.users.List(ctx, limit)
}

func (service *AccountService) UpdateUser(
	ctx context.Context,
	userID string,
	request dto.UpdateUserRequest,
) (account.User, error) {
	record, err := service.users.GetByID(ctx, userID)
	if err != nil {
		return account.User{}, err
	}
	if err := record.Apply(
		request.DisplayName, request.Role, request.Disabled, service.now(),
	); err != nil {
		return account.User{}, err
	}
	if err := service.users.Update(ctx, record); err != nil {
		return account.User{}, err
	}
	return record, nil
}

// DeleteUser removes a person. Nothing they created goes with them.
func (service *AccountService) DeleteUser(ctx context.Context, userID string) error {
	return service.users.Delete(ctx, userID)
}

// CreateKey mints a key and returns its secret exactly once. Only the digest
// reaches the database.
func (service *AccountService) CreateKey(
	ctx context.Context,
	request dto.CreateKeyRequest,
) (account.CreatedKey, error) {
	id, err := service.newID("key")
	if err != nil {
		return account.CreatedKey{}, err
	}
	record, err := account.NewKey(
		id, request.UserID, request.Name, request.Scopes, service.now(),
	)
	if err != nil {
		return account.CreatedKey{}, err
	}
	if record.UserID != "" {
		exists, err := service.users.Exists(ctx, record.UserID)
		if err != nil {
			return account.CreatedKey{}, err
		}
		if !exists {
			return account.CreatedKey{}, fmt.Errorf(
				"%w: key owner was not found", account.ErrInvalid,
			)
		}
	}
	prefix, secret, err := account.MintSecret()
	if err != nil {
		return account.CreatedKey{}, err
	}
	record.Prefix = prefix
	if err := service.keys.Create(ctx, record, account.HashSecret(secret)); err != nil {
		return account.CreatedKey{}, err
	}
	return account.CreatedKey{Key: record, Secret: secret}, nil
}

func (service *AccountService) ListKeys(
	ctx context.Context,
	limit int,
) ([]account.Key, error) {
	return service.keys.List(ctx, limit)
}

func (service *AccountService) RevokeKey(
	ctx context.Context,
	keyID string,
) (account.Key, error) {
	return service.keys.Revoke(ctx, keyID, service.now())
}

// AuthenticateContext resolves a presented API key to a principal. An unknown
// prefix and a wrong secret are both a plain false.
func (service *AccountService) AuthenticateContext(
	ctx context.Context,
	raw string,
) (auth.Principal, bool) {
	prefix, ok := account.SecretPrefix(raw)
	if !ok {
		return auth.Principal{}, false
	}
	credential, err := service.keys.CredentialByPrefix(ctx, prefix)
	if err != nil || !credential.Matches(raw) {
		return auth.Principal{}, false
	}
	return auth.Principal{
		Kind:   auth.KindAPIKey,
		KeyID:  credential.ID,
		Scopes: credential.Scopes,
	}, true
}

// CreateAdmin creates the first administrator and reports whether it made one.
//
// Boot calls this so an empty database has a way in. An existing username is
// not an error, it just means the admin is already there — but the password is
// never re-asserted, so one changed through the API is not silently reverted on
// the next restart.
func (service *AccountService) CreateAdmin(
	ctx context.Context,
	username string,
	password string,
) (bool, error) {
	_, err := service.CreateUser(ctx, dto.CreateUserRequest{
		Username: username, DisplayName: username,
		Role: "admin", Password: password,
	})
	if errors.Is(err, account.ErrConflict) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
