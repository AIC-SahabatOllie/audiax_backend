package usecase

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/config"
	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// fakeUserRepo keeps users in a map, so these tests need no database.
type fakeUserRepo struct {
	byID    map[string]*entity.User
	byEmail map[string]*entity.User
	updated map[string]any
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[string]*entity.User{}, byEmail: map[string]*entity.User{}}
}

func (f *fakeUserRepo) Create(_ *gorm.DB, user *entity.User) error {
	if _, exists := f.byEmail[user.Email]; exists {
		return gorm.ErrDuplicatedKey
	}
	copied := *user
	f.byID[user.ID] = &copied
	f.byEmail[user.Email] = &copied
	return nil
}

func (f *fakeUserRepo) FindByID(_ *gorm.DB, user *entity.User, id any) error {
	found, ok := f.byID[id.(string)]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	*user = *found
	return nil
}

func (f *fakeUserRepo) FindByEmail(_ *gorm.DB, user *entity.User, email string) error {
	found, ok := f.byEmail[email]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	*user = *found
	return nil
}

func (f *fakeUserRepo) Update(_ *gorm.DB, _ *entity.User, fields map[string]any) error {
	f.updated = fields
	return nil
}

type fakeSessionRepo struct {
	tokens       map[string]string
	created      int
	revokedForID string
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{tokens: map[string]string{}}
}

func (f *fakeSessionRepo) Create(_ context.Context, userID string, _ time.Duration) (string, error) {
	f.created++
	token := "token-" + userID
	f.tokens[token] = userID
	return token, nil
}

func (f *fakeSessionRepo) FindUserID(_ context.Context, token string) (string, error) {
	userID, ok := f.tokens[token]
	if !ok {
		return "", apperr.ErrUnauthorized
	}
	return userID, nil
}

func (f *fakeSessionRepo) Delete(_ context.Context, token string) error {
	delete(f.tokens, token)
	return nil
}

func (f *fakeSessionRepo) DeleteAllForUser(_ context.Context, userID string) error {
	f.revokedForID = userID
	for token, id := range f.tokens {
		if id == userID {
			delete(f.tokens, token)
		}
	}
	return nil
}

func newTestUseCase(t *testing.T) (*UserUseCase, *fakeUserRepo, *fakeSessionRepo) {
	t.Helper()

	// DummyDialector yields a *gorm.DB that supports WithContext without a server.
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	require.NoError(t, err)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	users, sessions := newFakeUserRepo(), newFakeSessionRepo()

	uc := NewUserUseCase(db, log, config.NewValidator(), users, sessions, time.Hour, bcrypt.MinCost)
	return uc, users, sessions
}

func TestRegisterReportsEveryInvalidField(t *testing.T) {
	uc, _, _ := newTestUseCase(t)

	_, err := uc.Register(context.Background(), &model.RegisterUserRequest{
		Email:    "not-an-email",
		Name:     "",
		Password: "short",
	})

	var validationErr *apperr.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "must be a valid email address", validationErr.Fields["email"])
	assert.Equal(t, "is required", validationErr.Fields["name"])
	assert.Equal(t, "must be at least 8 characters", validationErr.Fields["password"])
}

func TestRegisterNormalisesEmailAndHashesPassword(t *testing.T) {
	uc, users, _ := newTestUseCase(t)

	response, err := uc.Register(context.Background(), &model.RegisterUserRequest{
		Email:    "  Bryan@Example.COM ",
		Name:     "Bryan",
		Password: "correct-horse",
	})
	require.NoError(t, err)
	assert.Equal(t, "bryan@example.com", response.Email)

	stored := users.byEmail["bryan@example.com"]
	require.NotNil(t, stored)
	assert.NotEqual(t, "correct-horse", stored.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("correct-horse")))
}

func TestRegisterDuplicateEmailIsConflict(t *testing.T) {
	uc, _, _ := newTestUseCase(t)
	request := &model.RegisterUserRequest{Email: "dup@example.com", Name: "Dup", Password: "correct-horse"}

	_, err := uc.Register(context.Background(), request)
	require.NoError(t, err)

	_, err = uc.Register(context.Background(), request)
	assert.ErrorIs(t, err, apperr.ErrConflict)
}

func TestLoginUnknownEmailIsUnauthorizedAndIssuesNoSession(t *testing.T) {
	uc, _, sessions := newTestUseCase(t)

	_, err := uc.Login(context.Background(), &model.LoginUserRequest{
		Email:    "ghost@example.com",
		Password: "correct-horse",
	})

	assert.ErrorIs(t, err, apperr.ErrUnauthorized)
	assert.Zero(t, sessions.created)
}

func TestLoginWrongPasswordIsUnauthorized(t *testing.T) {
	uc, _, sessions := newTestUseCase(t)
	seedUser(t, uc)

	_, err := uc.Login(context.Background(), &model.LoginUserRequest{
		Email:    "bryan@example.com",
		Password: "wrong-password",
	})

	assert.ErrorIs(t, err, apperr.ErrUnauthorized)
	assert.Zero(t, sessions.created)
}

func TestLoginIssuesSessionAndVerifyResolvesIt(t *testing.T) {
	uc, users, _ := newTestUseCase(t)
	seedUser(t, uc)

	session, err := uc.Login(context.Background(), &model.LoginUserRequest{
		Email:    "bryan@example.com",
		Password: "correct-horse",
	})
	require.NoError(t, err)
	require.NotEmpty(t, session.Token)
	assert.True(t, session.ExpiresAt.After(time.Now()))

	userID, err := uc.Verify(context.Background(), session.Token)
	require.NoError(t, err)
	assert.Equal(t, users.byEmail["bryan@example.com"].ID, userID)
}

func TestVerifyRejectsMissingAndUnknownTokens(t *testing.T) {
	uc, _, _ := newTestUseCase(t)

	_, err := uc.Verify(context.Background(), "")
	assert.ErrorIs(t, err, apperr.ErrUnauthorized)

	_, err = uc.Verify(context.Background(), "made-up")
	assert.ErrorIs(t, err, apperr.ErrUnauthorized)
}

func TestLogoutInvalidatesTheToken(t *testing.T) {
	uc, _, _ := newTestUseCase(t)
	seedUser(t, uc)

	session, err := uc.Login(context.Background(), &model.LoginUserRequest{
		Email:    "bryan@example.com",
		Password: "correct-horse",
	})
	require.NoError(t, err)
	require.NoError(t, uc.Logout(context.Background(), session.Token))

	_, err = uc.Verify(context.Background(), session.Token)
	assert.ErrorIs(t, err, apperr.ErrUnauthorized)
}

func TestUpdatePasswordRevokesEverySession(t *testing.T) {
	uc, users, sessions := newTestUseCase(t)
	seedUser(t, uc)
	userID := users.byEmail["bryan@example.com"].ID

	session, err := uc.Login(context.Background(), &model.LoginUserRequest{
		Email:    "bryan@example.com",
		Password: "correct-horse",
	})
	require.NoError(t, err)

	newPassword := "a-brand-new-password"
	_, err = uc.Update(context.Background(), &model.UpdateUserRequest{UserID: userID, Password: &newPassword})
	require.NoError(t, err)

	assert.Equal(t, userID, sessions.revokedForID)

	_, err = uc.Verify(context.Background(), session.Token)
	assert.ErrorIs(t, err, apperr.ErrUnauthorized)

	require.NoError(t, bcrypt.CompareHashAndPassword(
		[]byte(users.updated["password_hash"].(string)), []byte(newPassword)))
}

func TestUpdateNameLeavesSessionsAlone(t *testing.T) {
	uc, users, sessions := newTestUseCase(t)
	seedUser(t, uc)
	userID := users.byEmail["bryan@example.com"].ID

	name := "Bryan T"
	_, err := uc.Update(context.Background(), &model.UpdateUserRequest{UserID: userID, Name: &name})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"name": name}, users.updated)
	assert.Empty(t, sessions.revokedForID)
}

func TestCurrentOnMissingUserIsNotFound(t *testing.T) {
	uc, _, _ := newTestUseCase(t)

	_, err := uc.Current(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func seedUser(t *testing.T, uc *UserUseCase) {
	t.Helper()
	_, err := uc.Register(context.Background(), &model.RegisterUserRequest{
		Email:    "bryan@example.com",
		Name:     "Bryan",
		Password: "correct-horse",
	})
	require.NoError(t, err)
}
