package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/flanksource/mission-control-plugins/arthas/internal/arthas"
)

const (
	SessionCreatePending = "pending"
	SessionCreateRunning = "running"
	SessionCreateFailed  = "failed"

	sessionCreateJobTTL = 30 * time.Minute
)

type SessionCreateJob struct {
	ID         string          `json:"jobId,omitempty"`
	TargetKey  string          `json:"targetKey,omitempty"`
	Status     string          `json:"status"`
	SessionID  string          `json:"sessionId,omitempty"`
	Session    *arthas.Session `json:"session,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

type SessionCreateJobRegistry struct {
	mu   sync.RWMutex
	jobs map[string]*SessionCreateJob
}

func NewSessionCreateJobRegistry() *SessionCreateJobRegistry {
	return &SessionCreateJobRegistry{jobs: make(map[string]*SessionCreateJob)}
}

func (r *SessionCreateJobRegistry) Start(targetKey string) (*SessionCreateJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	for _, job := range r.jobs {
		if job.TargetKey == targetKey && job.Status == SessionCreatePending {
			return cloneSessionCreateJob(job), false
		}
	}
	job := &SessionCreateJob{
		ID:        newJobID(),
		TargetKey: targetKey,
		Status:    SessionCreatePending,
		StartedAt: time.Now().UTC(),
	}
	r.jobs[job.ID] = job
	return cloneSessionCreateJob(job), true
}

func CompletedSessionCreateJob(targetKey string, session *arthas.Session) *SessionCreateJob {
	now := time.Now().UTC()
	job := &SessionCreateJob{
		TargetKey:  targetKey,
		Status:     SessionCreateRunning,
		Session:    session,
		StartedAt:  now,
		FinishedAt: &now,
	}
	if session != nil {
		job.SessionID = session.ID
		job.StartedAt = session.StartedAt
	}
	return job
}

func (r *SessionCreateJobRegistry) Succeed(id string, session *arthas.Session) {
	r.finish(id, func(job *SessionCreateJob) {
		job.Status = SessionCreateRunning
		job.Session = session
		if session != nil {
			job.SessionID = session.ID
		}
	})
}

func (r *SessionCreateJobRegistry) Fail(id string, err error) {
	r.finish(id, func(job *SessionCreateJob) {
		job.Status = SessionCreateFailed
		if err != nil {
			job.Error = err.Error()
		}
	})
}

func (r *SessionCreateJobRegistry) finish(id string, update func(*SessionCreateJob)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return
	}
	update(job)
	now := time.Now().UTC()
	job.FinishedAt = &now
}

func (r *SessionCreateJobRegistry) Get(id string) (*SessionCreateJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	job, ok := r.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneSessionCreateJob(job), true
}

func (r *SessionCreateJobRegistry) List() []*SessionCreateJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupLocked(time.Now())
	out := make([]*SessionCreateJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		out = append(out, cloneSessionCreateJob(job))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

func (r *SessionCreateJobRegistry) cleanupLocked(now time.Time) {
	for id, job := range r.jobs {
		if job.FinishedAt != nil && now.Sub(*job.FinishedAt) > sessionCreateJobTTL {
			delete(r.jobs, id)
		}
	}
}

func cloneSessionCreateJob(job *SessionCreateJob) *SessionCreateJob {
	if job == nil {
		return nil
	}
	copy := *job
	if job.FinishedAt != nil {
		finished := *job.FinishedAt
		copy.FinishedAt = &finished
	}
	return &copy
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("j%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
