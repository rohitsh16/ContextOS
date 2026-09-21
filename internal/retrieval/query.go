package retrieval

import (
	"context"
	"time"
)

// Mode represents the retrieval speed/thoroughness tradeoff profile.
type Mode string

const (
	ModeFast       Mode = "fast"
	ModeBalanced   Mode = "balanced"
	ModeExhaustive Mode = "exhaustive"
)

// Query encapsulates all parameters for a retrieval operation across code and state entities.
type Query struct {
	Task         string        `json:"task"`
	RepoID       string        `json:"repo_id"`
	Revision     string        `json:"revision"`
	WorkDir      string        `json:"work_dir"`
	ChangedFiles []string      `json:"changed_files"`
	Scope        string        `json:"scope"`
	MaxResults   int           `json:"max_results"`
	Deadline     time.Time     `json:"deadline"`
	Timeout      time.Duration `json:"timeout"`
	Mode         Mode          `json:"mode"`
}

// Context returns a bounded context.Context respecting the query's deadline/timeout.
func (q *Query) Context(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if !q.Deadline.IsZero() {
		return context.WithDeadline(parent, q.Deadline)
	}
	if q.Timeout > 0 {
		return context.WithTimeout(parent, q.Timeout)
	}
	return context.WithCancel(parent)
}
