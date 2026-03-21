package models

import "time"

type CLIType string

const (
	CLITypeBuiltin CLIType = "builtin"
	CLITypeNative  CLIType = "native"
	CLITypeGateway CLIType = "gateway"
	CLITypeSystem  CLIType = "system"
)

type CLI struct {
	Slug          string   `json:"slug"`
	DisplayName   string   `json:"displayName"`
	Summary       string   `json:"summary"`
	Type          CLIType  `json:"type"`
	Tags          []string `json:"tags"`
	HelpText      string   `json:"helpText"`
	VersionText   string   `json:"versionText"`
	Popularity    int      `json:"popularity"`
	RuntimeImage  string   `json:"runtimeImage"`
	Enabled       bool     `json:"enabled"`
	ExampleLine   string   `json:"exampleLine"`
	FavoriteCount int      `json:"favoriteCount"`
	CommentCount  int      `json:"commentCount"`
}

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
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

type LoginRequest struct {
	Username string `json:"username"`
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
