package plugin

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thegeeklab/wp-s3-action/aws"
)

func TestResultCollectorSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []JobResult
		want    Summary
	}{
		{
			name:    "empty collector reports no counters",
			results: nil,
			want:    Summary{},
		},
		{
			name: "counts one of each changed status",
			results: []JobResult{
				{Status: StatusAdded, Path: "a"},
				{Status: StatusModified, Path: "m"},
				{Status: StatusUpdated, Path: "u"},
				{Status: StatusDeleted, Path: "d"},
				{Status: StatusDownloaded, Path: "l"},
				{Status: StatusRedirected, Path: "r"},
			},
			want: Summary{
				Added:      1,
				Modified:   1,
				Updated:    1,
				Deleted:    1,
				Downloaded: 1,
				Redirected: 1,
			},
		},
		{
			name: "skipped results are counted but not retained as changes",
			results: []JobResult{
				{Status: StatusSkipped, Path: "s1"},
				{Status: StatusSkipped, Path: "s2"},
				{Status: StatusAdded, Path: "a"},
			},
			want: Summary{Added: 1, Skipped: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			collector := newResultCollector()
			collector.add(tt.results...)

			assert.Equal(t, tt.want, collector.summary())
		})
	}
}

func TestSummaryString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		summary Summary
		want    string
	}{
		{
			name:    "empty summary",
			summary: Summary{},
			want:    "no changes",
		},
		{
			name:    "single counter",
			summary: Summary{Added: 3},
			want:    "3 added",
		},
		{
			name:    "multiple counters in stable order",
			summary: Summary{Added: 3, Skipped: 5, Modified: 1},
			want:    "3 added, 1 modified, 5 skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.summary.String())
		})
	}
}

func TestResultStatusSymbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ResultStatus
		want   string
	}{
		{
			name:   "added",
			status: StatusAdded,
			want:   "+",
		},
		{
			name:   "modified",
			status: StatusModified,
			want:   "~",
		},
		{
			name:   "updated",
			status: StatusUpdated,
			want:   "*",
		},
		{
			name:   "deleted",
			status: StatusDeleted,
			want:   "-",
		},
		{
			name:   "downloaded",
			status: StatusDownloaded,
			want:   ">",
		},
		{
			name:   "redirected",
			status: StatusRedirected,
			want:   "→",
		},
		{
			name:   "unknown",
			status: ResultStatus("bogus"),
			want:   " ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.status.symbol(false))
		})
	}
}

func TestResultStatusColoredSymbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status ResultStatus
		want   string
	}{
		{
			name:   "added",
			status: StatusAdded,
			want:   "\033[32m+\033[0m",
		},
		{
			name:   "modified",
			status: StatusModified,
			want:   "\033[33m~\033[0m",
		},
		{
			name:   "updated",
			status: StatusUpdated,
			want:   "\033[33m*\033[0m",
		},
		{
			name:   "deleted",
			status: StatusDeleted,
			want:   "\033[31m-\033[0m",
		},
		{
			name:   "downloaded",
			status: StatusDownloaded,
			want:   "\033[36m>\033[0m",
		},
		{
			name:   "redirected",
			status: StatusRedirected,
			want:   "\033[36m→\033[0m",
		},
		{
			name:   "unknown",
			status: ResultStatus("bogus"),
			want:   " ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.status.symbol(true))
		})
	}
}

func TestResultCollectorRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []JobResult
		action  S3Action
		want    []string
	}{
		{
			name:    "no results renders only the summary",
			results: nil,
			action:  S3ActionUpload,
			want:    []string{"upload summary: no changes"},
		},
		{
			name: "changed results are sorted by path before the summary",
			results: []JobResult{
				{Status: StatusAdded, Path: "b.txt"},
				{Status: StatusDeleted, Path: "a.txt"},
				{Status: StatusSkipped, Path: "s.txt"},
			},
			action: S3ActionUpload,
			want: []string{
				"- a.txt",
				"+ b.txt",
				"upload summary: 1 added, 1 skipped, 1 deleted",
			},
		},
		{
			name: "equal paths are ordered by status for determinism",
			results: []JobResult{
				{Status: StatusDeleted, Path: "x"},
				{Status: StatusAdded, Path: "x"},
			},
			action: S3ActionDelete,
			want: []string{
				"+ x",
				"- x",
				"delete summary: 1 added, 1 deleted",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			collector := newResultCollector()
			collector.add(tt.results...)

			assert.Equal(t, tt.want, collector.renderColored(tt.action, false))
		})
	}
}

func TestUploadResultStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result aws.UploadResult
		want   ResultStatus
	}{
		{
			name:   "added",
			result: aws.UploadResultAdded,
			want:   StatusAdded,
		},
		{
			name:   "modified",
			result: aws.UploadResultModified,
			want:   StatusModified,
		},
		{
			name:   "updated",
			result: aws.UploadResultUpdated,
			want:   StatusUpdated,
		},
		{
			name:   "skipped",
			result: aws.UploadResultSkipped,
			want:   StatusSkipped,
		},
		{
			name:   "unknown falls back to skipped",
			result: aws.UploadResult("bogus"),
			want:   StatusSkipped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, uploadResultStatus(tt.result))
		})
	}
}

func TestColorsEnabled(t *testing.T) {
	tests := []struct {
		name       string
		setNoColor bool
		noColor    string
		setForce   bool
		forceColor string
		want       bool
	}{
		{
			name:       "NO_COLOR disables colors",
			setNoColor: true,
			noColor:    "1",
			want:       false,
		},
		{
			name:       "NO_COLOR empty string does not disable colors",
			setNoColor: true,
			noColor:    "",
			setForce:   true,
			forceColor: "1",
			want:       true,
		},
		{
			name:       "NO_COLOR takes precedence over FORCE_COLOR",
			setNoColor: true,
			noColor:    "1",
			setForce:   true,
			forceColor: "1",
			want:       false,
		},
		{
			name:       "FORCE_COLOR enables colors",
			setForce:   true,
			forceColor: "1",
			want:       true,
		},
		{
			name:       "FORCE_COLOR true enables colors",
			setForce:   true,
			forceColor: "true",
			want:       true,
		},
		{
			name:       "FORCE_COLOR 0 disables colors",
			setForce:   true,
			forceColor: "0",
			want:       false,
		},
		{
			name:       "FORCE_COLOR false disables colors",
			setForce:   true,
			forceColor: "false",
			want:       false,
		},
		{
			name:       "FORCE_COLOR FALSE disables colors case-insensitively",
			setForce:   true,
			forceColor: "FALSE",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setNoColor {
				t.Setenv("NO_COLOR", tt.noColor)
			} else {
				t.Setenv("NO_COLOR", "")
			}

			if tt.setForce {
				t.Setenv("FORCE_COLOR", tt.forceColor)
			} else {
				t.Setenv("FORCE_COLOR", "")
			}

			assert.Equal(t, tt.want, colorsEnabled())
		})
	}
}

func TestResultCollectorLog(t *testing.T) {
	tests := []struct {
		name    string
		results []JobResult
		action  S3Action
		want    string
	}{
		{
			name:    "no results writes only the summary",
			results: nil,
			action:  S3ActionUpload,
			want:    "upload summary: no changes\n",
		},
		{
			name: "changed results are written to output writer",
			results: []JobResult{
				{Status: StatusAdded, Path: "b.txt"},
				{Status: StatusDeleted, Path: "a.txt"},
			},
			action: S3ActionUpload,
			want:   "- a.txt\n+ b.txt\nupload summary: 1 added, 1 deleted\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "1")

			var buf bytes.Buffer

			collector := newResultCollector()
			collector.outputWriter = &buf
			collector.add(tt.results...)
			collector.log(tt.action)

			assert.Equal(t, tt.want, buf.String())
		})
	}
}
