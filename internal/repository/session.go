package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/constants"

	"github.com/redis/go-redis/v9"
)

// SessionRepository stores sessions in Redis rather than in a database table.
// That buys O(1) lookup on the hot auth path and expiry handled by Redis,
// instead of a full table scan per authenticated request plus a cleanup job.
type SessionRepository struct {
	client *redis.Client
}

func NewSessionRepository(client *redis.Client) *SessionRepository {
	return &SessionRepository{client: client}
}

// Create returns the plaintext token; only its SHA-256 digest is stored, so a
// leaked Redis dump cannot be replayed as a valid session.
func (r *SessionRepository) Create(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	key := sessionKey(token)

	// The per-user index is what makes "revoke every session" possible on a
	// password change. Redis expires the members; the index itself is given a
	// longer TTL so it outlives them.
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, key, userID, ttl)
	pipe.SAdd(ctx, userIndexKey(userID), key)
	pipe.Expire(ctx, userIndexKey(userID), ttl+constants.SessionIndexTTLMargin)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("store session: %w", err)
	}
	return token, nil
}

// FindUserID returns apperr.ErrUnauthorized when the token is unknown or expired.
func (r *SessionRepository) FindUserID(ctx context.Context, token string) (string, error) {
	userID, err := r.client.Get(ctx, sessionKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", apperr.ErrUnauthorized
	}
	if err != nil {
		return "", fmt.Errorf("read session: %w", err)
	}
	return userID, nil
}

func (r *SessionRepository) Delete(ctx context.Context, token string) error {
	if err := r.client.Del(ctx, sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	// ponytail: the index entry is left behind and reaped by DeleteAllForUser,
	// which tolerates already-expired members. Add an SREM here if index size
	// ever matters.
	return nil
}

// DeleteAllForUser revokes every session a user holds. Called on password
// change so a stolen token stops working the moment the password rotates.
func (r *SessionRepository) DeleteAllForUser(ctx context.Context, userID string) error {
	index := userIndexKey(userID)

	keys, err := r.client.SMembers(ctx, index).Result()
	if err != nil {
		return fmt.Errorf("read session index: %w", err)
	}

	pipe := r.client.TxPipeline()
	if len(keys) > 0 {
		pipe.Del(ctx, keys...)
	}
	pipe.Del(ctx, index)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}
	return nil
}

func sessionKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return constants.SessionKeyPrefix + hex.EncodeToString(digest[:])
}

func userIndexKey(userID string) string { return constants.UserSessionIndexPrefix + userID }
