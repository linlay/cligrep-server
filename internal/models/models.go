package models

import (
	"errors"
	"fmt"
	"time"
)

type CLIType string

const (
	CLITypeBuiltin CLIType = "builtin"
	CLITypeNative  CLIType = "native"
	CLITypeGateway CLIType = "gateway"
	CLITypeSystem  CLIType = "system"
	CLITypeText    CLIType = "text"
)

type EnvironmentKind string

const (
	EnvironmentKindSandbox EnvironmentKind = "SANDBOX"
	EnvironmentKindWebsite EnvironmentKind = "WEBSITE"
	EnvironmentKindText    EnvironmentKind = "TEXT"
)

type CLI struct {
	Slug            string          `json:"slug"`
	DisplayName     string          `json:"displayName"`
	Summary         string          `json:"summary"`
	Type            CLIType         `json:"type"`
	Tags            []string        `json:"tags"`
	HelpText        string          `json:"helpText"`
	VersionText     string          `json:"versionText"`
	Popularity      int             `json:"popularity"`
	RuntimeImage    string          `json:"runtimeImage"`
	Enabled         bool            `json:"enabled"`
	ExampleLine     string          `json:"exampleLine"`
	FavoriteCount   int             `json:"favoriteCount"`
	CommentCount    int             `json:"commentCount"`
	RunCount        int             `json:"runCount"`
	EnvironmentKind EnvironmentKind `json:"environmentKind"`
	SourceType      string          `json:"sourceType"`
	Author          string          `json:"author"`
	GitHubURL       string          `json:"githubUrl,omitempty"`
	GiteeURL        string          `json:"giteeUrl,omitempty"`
	License         string          `json:"license,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	OriginalCommand string          `json:"originalCommand,omitempty"`
	Executable      bool            `json:"executable"`
}

type CLIRelease struct {
	ID          int64             `json:"-"`
	Version     string            `json:"version"`
	PublishedAt time.Time         `json:"publishedAt"`
	IsCurrent   bool              `json:"isCurrent"`
	SourceKind  string            `json:"sourceKind"`
	SourceURL   string            `json:"sourceUrl"`
	Assets      []CLIReleaseAsset `json:"assets"`
}

type CLIReleaseAsset struct {
	ID          int64  `json:"-"`
	ReleaseID   int64  `json:"-"`
	FileName    string `json:"fileName"`
	DownloadURL string `json:"downloadUrl"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	PackageKind string `json:"packageKind"`
	ChecksumURL string `json:"checksumUrl"`
	SizeBytes   int64  `json:"sizeBytes"`
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	Email        string    `json:"email,omitempty"`
	AvatarURL    string    `json:"avatarUrl,omitempty"`
	AuthProvider string    `json:"authProvider"`
	IP           string    `json:"ip"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Comment struct {
	ID        int64     `json:"id"`
	CLISlug   string    `json:"cliSlug"`
	UserID    int64     `json:"userId"`
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

type GeneratedAsset struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	CLISlug   string    `json:"cliSlug,omitempty"`
	UserID    *int64    `json:"userId,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type ExecutionResult struct {
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"exitCode"`
	DurationMS   int64  `json:"durationMs"`
	Mode         string `json:"mode"`
	ResolvedCLI  string `json:"resolvedCli"`
	SessionState string `json:"sessionState,omitempty"`
}

type BuiltinExecResponse struct {
	Action        string           `json:"action"`
	Message       string           `json:"message"`
	SessionState  string           `json:"sessionState"`
	ResolvedCLI   string           `json:"resolvedCli,omitempty"`
	SearchResults []CLI            `json:"searchResults,omitempty"`
	Execution     *ExecutionResult `json:"execution,omitempty"`
	Asset         *GeneratedAsset  `json:"asset,omitempty"`
	User          *User            `json:"user,omitempty"`
	Hints         []string         `json:"hints,omitempty"`
}

type ExecRequest struct {
	CLISlug      string `json:"cliSlug"`
	Line         string `json:"line"`
	UserID       *int64 `json:"userId,omitempty"`
	ThemeContext string `json:"themeContext,omitempty"`
}

type BuiltinExecRequest struct {
	Line   string `json:"line"`
	UserID *int64 `json:"userId,omitempty"`
}

type LocalRegisterRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type LocalLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UpdateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

type SessionMetadata struct {
	IP        string
	UserAgent string
}

type FavoriteMutation struct {
	UserID  int64  `json:"userId"`
	CLISlug string `json:"cliSlug"`
	Active  bool   `json:"active"`
}

type CommentMutation struct {
	UserID  int64  `json:"userId"`
	CLISlug string `json:"cliSlug"`
	Body    string `json:"body"`
}

type AuthMethod string

const (
	AuthMethodGoogle        AuthMethod = "google"
	AuthMethodLocalPassword AuthMethod = "local_password"
)

type AuthResult string

const (
	AuthResultSuccess AuthResult = "success"
	AuthResultFailure AuthResult = "failure"
)

type AuthLoginLog struct {
	UserID        *int64
	Username      string
	DisplayName   string
	AuthMethod    AuthMethod
	LoginResult   AuthResult
	FailureReason string
	IP            string
	UserAgent     string
	LoginAt       time.Time
}

var (
	ErrUnauthorized       = errors.New("unauthorized")
	ErrAuthNotConfigured  = errors.New("auth is not configured")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameTaken      = errors.New("username is already taken")
	ErrInvalidUsername    = errors.New("username must match [a-zA-Z0-9_.-]{3,32}")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidDisplayName = errors.New("display name cannot be empty")
)

type AuthFailureReason string

const (
	AuthFailureReasonGoogleTokenExchangeFailed AuthFailureReason = "google_token_exchange_failed"
	AuthFailureReasonGoogleIDTokenMissing      AuthFailureReason = "google_id_token_missing"
	AuthFailureReasonGoogleIDTokenInvalid      AuthFailureReason = "google_id_token_invalid"
	AuthFailureReasonGoogleJWKSFetchFailed     AuthFailureReason = "google_jwks_fetch_failed"
	AuthFailureReasonGoogleUserUpsertFailed    AuthFailureReason = "google_user_upsert_failed"
	AuthFailureReasonGoogleSessionCreateFailed AuthFailureReason = "google_session_create_failed"
	AuthFailureReasonGoogleCallbackFailed      AuthFailureReason = "google_callback_failed"
)

type AuthFailureError struct {
	Reason AuthFailureReason
	Err    error
}

func (e *AuthFailureError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *AuthFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewAuthFailureError(reason AuthFailureReason, err error) error {
	return &AuthFailureError{
		Reason: reason,
		Err:    err,
	}
}

func AuthFailureReasonFromError(err error) AuthFailureReason {
	var authErr *AuthFailureError
	if errors.As(err, &authErr) && authErr != nil && authErr.Reason != "" {
		return authErr.Reason
	}
	return AuthFailureReasonGoogleCallbackFailed
}
