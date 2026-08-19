// Package constants is the single home for every named constant in the
// project. If you are about to write `const` anywhere else, write it here
// instead and import it.
//
// This package must never import another package from this module. Everything
// imports it, so any dependency of its own risks an import cycle. Standard
// library only, and in practice only `time`.
package constants

import "time"

// Redis key prefixes. Changing one invalidates every live session or index
// under the old prefix, so treat these as a wire format, not a preference.
const (
	// SessionKeyPrefix is followed by the hex SHA-256 digest of a session token.
	SessionKeyPrefix = "session:"
	// UserSessionIndexPrefix is followed by a user id and holds the set of that
	// user's session keys, which is what makes bulk revocation possible.
	UserSessionIndexPrefix = "user_sessions:"
)

// Keys written into fiber's per-request Locals store.
const (
	// AuthLocalsKey holds a *model.Auth, set by the auth middleware.
	AuthLocalsKey = "auth"
	// RequestIDLocalsKey is set by fiber's requestid middleware. The name is
	// dictated by that middleware and cannot be chosen freely.
	RequestIDLocalsKey = "requestid"
)

// AuthScheme is the Authorization header scheme the API accepts.
const AuthScheme = "Bearer"

// BcryptMaxPasswordBytes is the point past which bcrypt silently ignores input.
// Passwords are rejected above this length rather than truncated.
const BcryptMaxPasswordBytes = 72

// HTTP server limits.
const (
	HTTPReadTimeout  = 15 * time.Second
	HTTPWriteTimeout = 30 * time.Second
	HTTPIdleTimeout  = 60 * time.Second
	HTTPBodyLimit    = 4 * 1024 * 1024
)

// Timeouts applied while establishing infrastructure connections at startup.
const (
	DatabasePingTimeout = 10 * time.Second
	RedisPingTimeout    = 10 * time.Second
)

// SupabaseTransactionPoolerPort identifies Supabase's transaction pooler.
// That pooler multiplexes connections and cannot hold server-side prepared
// statements, so detecting it switches pgx to the simple protocol.
const SupabaseTransactionPoolerPort = ":6543"

// SessionIndexTTLMargin keeps the per-user session index alive slightly longer
// than the sessions it points at, so the last member never outlives its index.
const SessionIndexTTLMargin = time.Hour

// Defaults applied when the matching environment variable is unset.
const (
	DefaultAppName  = "audiax"
	DefaultAppEnv   = "development"
	DefaultPort     = 3000
	DefaultLogLevel = "info"

	DefaultDBMaxIdleConns  = 5
	DefaultDBMaxOpenConns  = 20
	DefaultDBConnMaxLife   = 30 * time.Minute
	DefaultDBSlowThreshold = 200 * time.Millisecond

	DefaultSessionTTL      = 7 * 24 * time.Hour
	DefaultBcryptCost      = 12
	DefaultShutdownTimeout = 15 * time.Second
	DefaultCORSOrigins     = "*"
)

// EnvProduction is the APP_ENV value that switches on production behaviour:
// JSON logs, no stack traces in responses.
const EnvProduction = "production"
