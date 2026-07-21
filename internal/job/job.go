package job

import "github.com/semsemyonoff/cueBreaker-backend/internal/joblog"

// Status represents where a job currently is in the split pipeline.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusSplitting Status = "splitting"
	StatusTagging   Status = "tagging"
	StatusDone      Status = "done"
	StatusError     Status = "error"
)

// active reports whether jobs in this status still occupy their registry
// slot (queued or running), and so must block a duplicate Enqueue.
func (s Status) active() bool {
	return s == StatusQueued || s == StatusSplitting || s == StatusTagging
}

// State is a job's current, JSON-serializable status snapshot.
type State struct {
	Status          Status
	Message         string
	ResultFiles     []string
	ProgressCurrent int
	ProgressTotal   int
	ProgressDetail  string

	// Log holds this job's synthesized pipeline events. It is not serialized
	// directly — the server handler reads it into the wire response, since the
	// buffer is a shared pointer rather than a value to copy per State.
	Log *joblog.Buffer
}

// JobID derives the job registry key from a scan-relative directory path
// and CUE file name, matching app.py's "<path>/<cue_file>" convention.
func JobID(path, cueFile string) string {
	return path + "/" + cueFile
}
