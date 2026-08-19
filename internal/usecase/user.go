package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/entity"
	"audiax/internal/model"
	"audiax/internal/model/converter"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// The interfaces below are declared here, at the consumer, so this package can
// be unit tested with fakes and never imports a concrete repository.

type UserRepository interface {
	Create(db *gorm.DB, user *entity.User) error
	FindByID(db *gorm.DB, user *entity.User, id any) error
	FindByEmail(db *gorm.DB, user *entity.User, email string) error
	Update(db *gorm.DB, user *entity.User, fields map[string]any) error
}

type SessionRepository interface {
	Create(ctx context.Context, userID string, ttl time.Duration) (string, error)
	FindUserID(ctx context.Context, token string) (string, error)
	Delete(ctx context.Context, token string) error
	DeleteAllForUser(ctx context.Context, userID string) error
}

type UserUseCase struct {
	db         *gorm.DB
	log        *slog.Logger
	validate   *validator.Validate
	users      UserRepository
	sessions   SessionRepository
	sessionTTL time.Duration
	bcryptCost int
	dummyHash  []byte
}

func NewUserUseCase(db *gorm.DB, log *slog.Logger, validate *validator.Validate,
	users UserRepository, sessions SessionRepository, sessionTTL time.Duration, bcryptCost int) *UserUseCase {

	// Compared against when an email is unknown, so a login attempt costs the
	// same whether or not the account exists. Without it, response time leaks
	// which emails are registered.
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("audiax-timing-equaliser"), bcryptCost)
	if err != nil {
		panic(fmt.Sprintf("invalid bcrypt cost %d: %v", bcryptCost, err))
	}

	return &UserUseCase{
		db:         db,
		log:        log,
		validate:   validate,
		users:      users,
		sessions:   sessions,
		sessionTTL: sessionTTL,
		bcryptCost: bcryptCost,
		dummyHash:  dummyHash,
	}
}

func (u *UserUseCase) Register(ctx context.Context, request *model.RegisterUserRequest) (*model.UserResponse, error) {
	// Normalise before validating: a pasted address with a stray space is a
	// typo to clean up, not a request to reject.
	request.Email = normaliseEmail(request.Email)
	request.Name = strings.TrimSpace(request.Name)

	if err := u.validateStruct(request); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), u.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &entity.User{
		ID:           uuid.NewString(),
		Email:        request.Email,
		Name:         request.Name,
		PasswordHash: string(hash),
	}

	// No pre-flight "does this email exist" query: it cannot close the race
	// between two concurrent signups anyway. The unique index is the real
	// guard, so let the insert fail and translate it.
	if err := u.users.Create(u.db.WithContext(ctx), user); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperr.ErrConflict
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	u.log.InfoContext(ctx, "user registered", "user_id", user.ID)
	return converter.UserToResponse(user), nil
}

func (u *UserUseCase) Login(ctx context.Context, request *model.LoginUserRequest) (*model.SessionResponse, error) {
	request.Email = normaliseEmail(request.Email)

	if err := u.validateStruct(request); err != nil {
		return nil, err
	}

	user := new(entity.User)
	err := u.users.FindByEmail(u.db.WithContext(ctx), user, request.Email)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		_ = bcrypt.CompareHashAndPassword(u.dummyHash, []byte(request.Password))
		return nil, apperr.ErrUnauthorized
	case err != nil:
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
		return nil, apperr.ErrUnauthorized
	}

	token, err := u.sessions.Create(ctx, user.ID, u.sessionTTL)
	if err != nil {
		return nil, err
	}

	u.log.InfoContext(ctx, "user logged in", "user_id", user.ID)
	return &model.SessionResponse{
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(u.sessionTTL),
	}, nil
}

// Verify resolves a session token to a user id. It touches Redis only, which
// is why the auth middleware costs no database round trip.
func (u *UserUseCase) Verify(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", apperr.ErrUnauthorized
	}
	return u.sessions.FindUserID(ctx, token)
}

func (u *UserUseCase) Current(ctx context.Context, userID string) (*model.UserResponse, error) {
	user := new(entity.User)
	if err := u.users.FindByID(u.db.WithContext(ctx), user, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return converter.UserToResponse(user), nil
}

func (u *UserUseCase) Update(ctx context.Context, request *model.UpdateUserRequest) (*model.UserResponse, error) {
	if err := u.validateStruct(request); err != nil {
		return nil, err
	}

	db := u.db.WithContext(ctx)

	user := new(entity.User)
	if err := u.users.FindByID(db, user, request.UserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	fields := map[string]any{}
	if request.Name != nil {
		fields["name"] = strings.TrimSpace(*request.Name)
	}
	if request.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*request.Password), u.bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		fields["password_hash"] = string(hash)
	}
	if len(fields) == 0 {
		return converter.UserToResponse(user), nil
	}

	// A multi-statement write would wrap this in u.db.Transaction(func(tx *gorm.DB) error {...})
	// and pass tx down instead of db. One statement needs no transaction.
	if err := u.users.Update(db, user, fields); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if request.Password != nil {
		// A rotated password must not leave old tokens usable.
		if err := u.sessions.DeleteAllForUser(ctx, user.ID); err != nil {
			return nil, err
		}
		u.log.InfoContext(ctx, "sessions revoked after password change", "user_id", user.ID)
	}

	return converter.UserToResponse(user), nil
}

func (u *UserUseCase) Logout(ctx context.Context, token string) error {
	return u.sessions.Delete(ctx, token)
}

// validateStruct turns validator's error list into an apperr.ValidationError so
// the client is told which field failed and why.
func (u *UserUseCase) validateStruct(request any) error {
	err := u.validate.Struct(request)
	if err == nil {
		return nil
	}

	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return fmt.Errorf("validate: %w", err)
	}

	var fieldErrors validator.ValidationErrors
	if !errors.As(err, &fieldErrors) {
		return fmt.Errorf("validate: %w", err)
	}

	fields := make(map[string]string, len(fieldErrors))
	for _, fe := range fieldErrors {
		fields[fe.Field()] = describe(fe)
	}
	return &apperr.ValidationError{Fields: fields}
}

func describe(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	default:
		return "failed the " + fe.Tag() + " rule"
	}
}

func normaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
