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
	// Must exceed MaxAudioUploadBytes plus multipart framing. At the old 4 MiB
	// a 120-second calibration clip (3.84 MB of 16 kHz 16-bit mono) left 354 KB
	// of headroom and none at all for a phone recording at 44.1 kHz or stereo:
	// fiber rejected the upload before any handler ran.
	HTTPBodyLimit = 20 * 1024 * 1024
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

	// A cold CPU forward pass over ~119 calibration windows dominates this.
	// Measured ~1.24s/window on the dockerized service (4 CPU / 4G limit), so
	// ~119 windows alone is already ~148s -- 120s was cutting a real 2-minute
	// calibration recording close. 240s leaves headroom for a colder cache.
	DefaultAITimeout     = 240 * time.Second
	DefaultStorageBucket = "audiax-audio"
)

// EnvProduction is the APP_ENV value that switches on production behaviour:
// JSON logs, no stack traces in responses.
const EnvProduction = "production"

// Health card statuses, mirroring ai/decision.py. These are a wire format
// shared with the AI service: renaming one breaks the CHECK constraint on
// inspections.status and every stored row that used the old spelling.
const (
	StatusNormal       = "NORMAL"
	StatusWarning      = "WARNING"
	StatusCritical     = "CRITICAL"
	StatusUncalibrated = "KALIBRASI_KURANG"
)

// Calibration quality values, mirroring ai/calibration.py.
const (
	CalibrationQualityGood = "baik"
	CalibrationQualityLow  = "rendah"
)

// EmbeddingDtypeFloat16 is the only dtype MachineBaseline emits. The size CHECK
// on baselines assumes 2 bytes per element, so a different dtype must fail loudly.
const EmbeddingDtypeFloat16 = "float16"

// TriageDisclaimer mirrors DISCLAIMER in ai/decision.py. It is not stored per
// inspection (docs/erd.md §6); the backend attaches it when building a response.
// Live inspections echo the value the AI service returned; history rows use this
// constant, so the two must stay identical.
const TriageDisclaimer = "Alat bantu triase, bukan diagnosis mengikat -- tetap perlu inspeksi teknisi."

// Audio upload handling.
const (
	// AudioFormField is the multipart field name both this API and the AI
	// service use for the audio file.
	AudioFormField = "audio"
	// MaxAudioUploadBytes bounds a single upload. 120 s of 16 kHz 16-bit mono
	// is 3.84 MB; the margin covers phones that record at 44.1 kHz or stereo.
	MaxAudioUploadBytes = 16 * 1024 * 1024
	// Object key prefixes inside the storage bucket.
	CalibrationAudioPrefix = "calibrations/"
	InspectionAudioPrefix  = "inspections/"
	// AudioContentType is what the storage client declares on upload.
	AudioContentType = "audio/wav"
	// StorageTimeout bounds one object upload. Deliberately far shorter than
	// DefaultAITimeout: a PUT of a few megabytes that has not finished in 30 s
	// is not going to, and a user is already waiting on the request.
	StorageTimeout = 30 * time.Second
)

// AI service endpoints, from audiax_model/service/main.py.
const (
	AICalibratePath = "/v1/calibrate"
	AIInspectPath   = "/v1/inspect"
	AIHealthPath    = "/healthz"
	// AIBaselineFormField is the form field /v1/inspect expects the serialised
	// baseline in.
	AIBaselineFormField = "baseline_json"
	// AIMachineLabelFormField is the form field /v1/calibrate expects the
	// operator's machine label in.
	AIMachineLabelFormField = "machine_label"
)

// InspectionHistoryLimit caps how many inspections one history request returns.
const InspectionHistoryLimit = 100

// Advisory layer ("Teknisi Saku"): the language-model advisory that explains
// a HealthCard and answers operator follow-up questions. See
// internal/advisory/PROMPT_CONTRACT.md for the prompt contract this bounds.
const (
	// AdvisoryMaxHistoryTurns caps how many trailing conversation turns are
	// rendered into the prompt. Must match prompt_template.txt's render rules
	// exactly: the same template is used to build training corpus (Track B)
	// and to serve (Track A), and a mismatch here is a silent train/serve skew.
	AdvisoryMaxHistoryTurns = 8
)

// AdvisoryForbiddenDiagnosisPhrases fails guard.go's diagnosis check
// (DESIGN.md §4.2) when any appears in an LLM completion, case-insensitively.
// This tool triages; it never names a fault or a root cause.
var AdvisoryForbiddenDiagnosisPhrases = []string{
	"bearing aus",
	"bearing rusak",
	"impeler pecah",
	"motor terbakar",
	"kerusakan pada",
	"disebabkan oleh",
	"sisa umur",
	"akan rusak dalam",
}
