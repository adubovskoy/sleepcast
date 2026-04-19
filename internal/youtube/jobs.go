package youtube

import (
	"context"
	"sync"
	"time"
)

type JobState string

const (
	StatePending JobState = "pending"
	StateReady   JobState = "ready"
	StateError   JobState = "error"
)

type JobStatus struct {
	State   JobState `json:"state"`
	Error   string   `json:"error,omitempty"`
	Updated int64    `json:"updated"`
}

type JobTracker struct {
	mu   sync.Mutex
	jobs map[string]*JobStatus
}

func NewJobTracker() *JobTracker {
	return &JobTracker{jobs: make(map[string]*JobStatus)}
}

func (t *JobTracker) Get(videoID string) (JobStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.jobs[videoID]
	if !ok {
		return JobStatus{}, false
	}
	return *s, true
}

// Start returns true if the caller should do the work; false if a job is already running.
func (t *JobTracker) Start(videoID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.jobs[videoID]; ok && s.State == StatePending {
		return false
	}
	t.jobs[videoID] = &JobStatus{State: StatePending, Updated: time.Now().Unix()}
	return true
}

func (t *JobTracker) finish(videoID string, state JobState, errMsg string) {
	t.mu.Lock()
	t.jobs[videoID] = &JobStatus{State: state, Error: errMsg, Updated: time.Now().Unix()}
	t.mu.Unlock()

	go func() {
		time.Sleep(60 * time.Second)
		t.mu.Lock()
		delete(t.jobs, videoID)
		t.mu.Unlock()
	}()
}

func (t *JobTracker) Run(ctx context.Context, videoID string, fn func(context.Context) error) {
	go func() {
		err := fn(ctx)
		if err != nil {
			t.finish(videoID, StateError, err.Error())
			return
		}
		t.finish(videoID, StateReady, "")
	}()
}
