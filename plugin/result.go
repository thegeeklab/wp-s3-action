package plugin

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// ResultStatus describes the outcome of a single processed job.
type ResultStatus string

const (
	StatusAdded      ResultStatus = "added"
	StatusModified   ResultStatus = "modified"
	StatusUpdated    ResultStatus = "updated"
	StatusSkipped    ResultStatus = "skipped"
	StatusDeleted    ResultStatus = "deleted"
	StatusDownloaded ResultStatus = "downloaded"
	StatusRedirected ResultStatus = "redirected"
)

// JobResult captures the outcome of a processed job for the run summary
// and diff listing.
type JobResult struct {
	Status ResultStatus
	Path   string
}

// Summary aggregates the outcomes of all jobs in a single action run.
type Summary struct {
	Added      int
	Modified   int
	Updated    int
	Skipped    int
	Deleted    int
	Downloaded int
	Redirected int
}

// String renders the summary in a stable order and reports "no changes"
// when every counter is zero.
func (s Summary) String() string {
	parts := make([]string, 0)

	if s.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", s.Added))
	}

	if s.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", s.Modified))
	}

	if s.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", s.Updated))
	}

	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}

	if s.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", s.Deleted))
	}

	if s.Downloaded > 0 {
		parts = append(parts, fmt.Sprintf("%d downloaded", s.Downloaded))
	}

	if s.Redirected > 0 {
		parts = append(parts, fmt.Sprintf("%d redirected", s.Redirected))
	}

	if len(parts) == 0 {
		return "no changes"
	}

	return strings.Join(parts, ", ")
}

// symbol returns the single-character marker used in the diff listing for
// each status.
func (s ResultStatus) symbol() string {
	switch s {
	case StatusAdded:
		return "+"
	case StatusModified:
		return "~"
	case StatusUpdated:
		return "*"
	case StatusDeleted:
		return "-"
	case StatusDownloaded:
		return ">"
	case StatusRedirected:
		return "→"
	default:
		return " "
	}
}

// resultCollector gathers job results from concurrent workers. Skipped
// results are only counted to keep memory bounded on large syncs, while
// every other outcome is retained for the diff listing.
type resultCollector struct {
	mu      sync.Mutex
	changed []JobResult
	skipped atomic.Int64
}

func newResultCollector() *resultCollector {
	return &resultCollector{}
}

func (c *resultCollector) add(results ...JobResult) {
	for _, r := range results {
		if r.Status == StatusSkipped {
			c.skipped.Add(1)

			continue
		}

		c.mu.Lock()
		c.changed = append(c.changed, r)
		c.mu.Unlock()
	}
}

func (c *resultCollector) summary() Summary {
	var s Summary

	for _, r := range c.changed {
		switch r.Status {
		case StatusAdded:
			s.Added++
		case StatusModified:
			s.Modified++
		case StatusUpdated:
			s.Updated++
		case StatusDeleted:
			s.Deleted++
		case StatusDownloaded:
			s.Downloaded++
		case StatusRedirected:
			s.Redirected++
		}
	}

	s.Skipped = int(c.skipped.Load())

	return s
}

func (c *resultCollector) sortedChanged() []JobResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	changed := slices.Clone(c.changed)
	slices.SortFunc(changed, func(a, b JobResult) int {
		if n := cmp.Compare(a.Path, b.Path); n != 0 {
			return n
		}

		return cmp.Compare(string(a.Status), string(b.Status))
	})

	return changed
}

// render produces the diff listing lines for changed files followed by the
// summary line, so callers can either log or test the output without being
// coupled to zerolog.
func (c *resultCollector) render(action S3Action) []string {
	lines := make([]string, 0)

	for _, r := range c.sortedChanged() {
		lines = append(lines, fmt.Sprintf("%s %s", r.Status.symbol(), r.Path))
	}

	lines = append(lines, fmt.Sprintf("%s summary: %s", action, c.summary()))

	return lines
}

func (c *resultCollector) log(action S3Action) {
	for _, line := range c.render(action) {
		log.Info().Msg(line)
	}
}
