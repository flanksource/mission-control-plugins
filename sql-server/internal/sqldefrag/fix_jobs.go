package sqldefrag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

type FixJob struct {
	ID         string             `json:"id"`
	Status     JobStatus          `json:"status"`
	Database   string             `json:"database"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt *time.Time         `json:"finishedAt,omitempty"`
	Duration   time.Duration      `json:"duration,omitempty"`
	Error      string             `json:"error,omitempty"`
	Fixes      []Fix              `json:"fixes,omitempty"`
	Results    []FixResult        `json:"results,omitempty"`
	Summary    FixJobSummary      `json:"summary"`
	Cancel     context.CancelFunc `json:"-"`
}

type FixJobSummary struct {
	Total       int `json:"total"`
	Applied     int `json:"applied"`
	Failed      int `json:"failed"`
	Rebuilds    int `json:"rebuilds"`
	Reorganizes int `json:"reorganizes"`
	UpdateStats int `json:"updateStats"`
	DropIndexes int `json:"dropIndexes"`
}

type BulkRebuildOptions struct {
	Database       string     `json:"database,omitempty"`
	Tables         []TableRef `json:"tables"`
	RebuildIndexes bool       `json:"rebuildIndexes"`
	UpdateStats    bool       `json:"updateStats"`
	Offline        bool       `json:"offline,omitempty"`
	Resumable      bool       `json:"resumable,omitempty"`
	MaxDop         int        `json:"maxDop,omitempty"`
	StatsSample    string     `json:"statsSample,omitempty"`
}

type FixJobRegistry struct {
	mu   sync.Mutex
	jobs map[string]*FixJob
}

func NewFixJobRegistry() *FixJobRegistry {
	return &FixJobRegistry{jobs: map[string]*FixJob{}}
}

func (r *FixJobRegistry) StartWithDB(db *gorm.DB, database string, fixes []Fix) (*FixJob, error) {
	if r == nil {
		return nil, fmt.Errorf("fix job registry is not configured")
	}
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if len(fixes) == 0 {
		return nil, fmt.Errorf("fixes must not be empty")
	}
	resolved, err := resolveDatabase(context.Background(), db, database)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, fmt.Errorf("apply fixes requires a single database; 'all' is not supported")
	}
	ordered := orderFixes(fixes)
	for i, f := range ordered {
		if err := validateFixForApply(f); err != nil {
			return nil, fmt.Errorf("fix %d: %w", i+1, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &FixJob{
		ID:        fmt.Sprintf("defrag-fix-%d", time.Now().UnixNano()),
		Status:    JobRunning,
		Database:  resolved,
		StartedAt: time.Now(),
		Fixes:     append([]Fix(nil), ordered...),
		Summary:   FixJobSummary{Total: len(ordered)},
		Cancel:    cancel,
	}
	r.mu.Lock()
	r.jobs[job.ID] = job
	r.pruneLocked(25)
	r.mu.Unlock()

	go r.runDetached(ctx, db, job.ID, resolved, ordered)
	return job.Clone(), nil
}

func (r *FixJobRegistry) StartBulkWithDB(db *gorm.DB, opts BulkRebuildOptions) (*FixJob, error) {
	if len(opts.Tables) == 0 {
		return nil, fmt.Errorf("tables must not be empty")
	}
	if !opts.RebuildIndexes && !opts.UpdateStats {
		return nil, fmt.Errorf("select at least one of rebuildIndexes or updateStats")
	}
	sample, err := ParseSampleMode(opts.StatsSample)
	if err != nil {
		return nil, err
	}
	rebuild := RebuildOptions{Offline: opts.Offline, Resumable: opts.Resumable, MaxDop: opts.MaxDop}
	if !rebuild.Offline {
		online, _, err := supportsOnlineRebuild(context.Background(), db)
		if err != nil {
			return nil, err
		}
		if !online {
			rebuild.Offline = true
		}
	}
	fixes := BulkRebuildFixes(opts.Tables, opts.RebuildIndexes, opts.UpdateStats, rebuild, sample)
	if len(fixes) == 0 {
		return nil, fmt.Errorf("no fixes generated for the selected tables")
	}
	return r.StartWithDB(db, opts.Database, fixes)
}

func (r *FixJobRegistry) StartRollbackRestoreWithDB(db *gorm.DB, database string, id int64) (*FixJob, error) {
	if r == nil {
		return nil, fmt.Errorf("fix job registry is not configured")
	}
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if id <= 0 {
		return nil, fmt.Errorf("id must be a positive audit entry id")
	}
	resolved, err := resolveDatabase(context.Background(), db, database)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, fmt.Errorf("restore requires a single database; 'all' is not supported")
	}
	entry, err := NewAuditLog(db).Get(context.Background(), resolved, id)
	if err != nil {
		return nil, err
	}
	if entry.Action != AuditDropIndex {
		return nil, fmt.Errorf("audit entry %d is %q, not %q", id, entry.Action, AuditDropIndex)
	}
	if entry.RollbackSQL == "" {
		return nil, fmt.Errorf("audit entry %d has no rollback SQL", id)
	}
	ctx, cancel := context.WithCancel(context.Background())
	fix := Fix{
		Kind:   FixRestoreIndex,
		Schema: entry.SchemaName,
		Table:  entry.Table,
		Target: entry.ObjectName,
		Detail: fmt.Sprintf("restore dropped index from audit #%d", entry.ID),
		SQL:    entry.RollbackSQL,
	}
	job := &FixJob{
		ID:        fmt.Sprintf("defrag-rollback-restore-%d", time.Now().UnixNano()),
		Status:    JobRunning,
		Database:  resolved,
		StartedAt: time.Now(),
		Fixes:     []Fix{fix},
		Summary:   FixJobSummary{Total: 1},
		Cancel:    cancel,
	}
	r.mu.Lock()
	r.jobs[job.ID] = job
	r.pruneLocked(25)
	r.mu.Unlock()

	go r.runRestoreDetached(ctx, db, job.ID, resolved, entry, fix)
	return job.Clone(), nil
}

func (r *FixJobRegistry) runDetached(ctx context.Context, db *gorm.DB, id, database string, fixes []Fix) {
	for _, f := range fixes {
		if err := ctx.Err(); err != nil {
			r.finish(id, JobStopped, err.Error())
			return
		}
		res := applyOneFix(ctx, db, database, f)
		r.appendResult(id, res)
	}

	r.mu.Lock()
	job, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	if job.Status == JobStopped {
		job.Cancel = nil
		r.mu.Unlock()
		return
	}
	status := JobDone
	errMsg := ""
	if job.Summary.Failed > 0 {
		status = JobFailed
		errMsg = fmt.Sprintf("%d fix(es) failed", job.Summary.Failed)
	}
	r.mu.Unlock()
	r.finish(id, status, errMsg)
}

func (r *FixJobRegistry) runRestoreDetached(ctx context.Context, db *gorm.DB, id, database string, entry AuditEntry, fix Fix) {
	res := FixResult{Fix: fix, Messages: []string{entry.RollbackSQL}}
	if err := ctx.Err(); err != nil {
		res.Error = err.Error()
		r.appendResult(id, res)
		r.finish(id, JobStopped, err.Error())
		return
	}
	runErr := db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		return tx.Exec(entry.RollbackSQL).Error
	})
	if runErr != nil {
		res.Error = runErr.Error()
		r.appendResult(id, res)
		r.finish(id, JobFailed, runErr.Error())
		return
	}
	res.Applied = true
	audit := NewAuditLog(db)
	if err := audit.RecordRestore(ctx, database, entry); err != nil {
		msg := fmt.Sprintf("restored index but failed to record RESTORE audit entry: %v", err)
		res.Error = msg
		res.Messages = append(res.Messages, msg)
	}
	if err := audit.MarkRestored(ctx, database, entry.ID); err != nil {
		msg := fmt.Sprintf("restored index but failed to stamp audit #%d as restored: %v", entry.ID, err)
		res.Error = msg
		res.Messages = append(res.Messages, msg)
	}
	r.appendResult(id, res)
	if res.Error != "" {
		r.finish(id, JobFailed, res.Error)
		return
	}
	r.finish(id, JobDone, "")
}

func (r *FixJobRegistry) appendResult(id string, res FixResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return
	}
	job.Results = append(job.Results, res)
	if res.Applied {
		job.Summary.Applied++
	}
	if !res.Applied || res.Error != "" {
		job.Summary.Failed++
	}
	switch res.Fix.Kind {
	case FixRebuild, FixRebuildAll:
		job.Summary.Rebuilds++
	case FixReorganize:
		job.Summary.Reorganizes++
	case FixUpdateStats, FixUpdateAllStats:
		job.Summary.UpdateStats++
	case FixDropIndex:
		job.Summary.DropIndexes++
	}
}

func (r *FixJobRegistry) finish(id string, status JobStatus, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return
	}
	finished := time.Now()
	job.Status = status
	job.FinishedAt = &finished
	job.Duration = finished.Sub(job.StartedAt)
	job.Cancel = nil
	job.Error = errMsg
}

func (r *FixJobRegistry) List() []*FixJob {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*FixJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		out = append(out, job.Clone())
	}
	return out
}

func (r *FixJobRegistry) Get(id string) (*FixJob, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, false
	}
	return job.Clone(), true
}

func (r *FixJobRegistry) Stop(id string) (*FixJob, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[id]
	if !ok {
		return nil, false
	}
	stopFixJobLocked(job)
	return job.Clone(), true
}

func (r *FixJobRegistry) StopRunning() []*FixJob {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var stopped []*FixJob
	for _, job := range r.jobs {
		if job.Status != JobRunning {
			continue
		}
		stopFixJobLocked(job)
		stopped = append(stopped, job.Clone())
	}
	return stopped
}

func stopFixJobLocked(job *FixJob) {
	if job.Status != JobRunning {
		return
	}
	finished := time.Now()
	job.Status = JobStopped
	job.FinishedAt = &finished
	job.Duration = finished.Sub(job.StartedAt)
	if job.Cancel != nil {
		job.Cancel()
		job.Cancel = nil
	}
}

func (r *FixJobRegistry) pruneLocked(keep int) {
	if len(r.jobs) <= keep {
		return
	}
	type candidate struct {
		id string
		at time.Time
	}
	var done []candidate
	for id, job := range r.jobs {
		if job.Status == JobRunning {
			continue
		}
		at := job.StartedAt
		if job.FinishedAt != nil {
			at = *job.FinishedAt
		}
		done = append(done, candidate{id: id, at: at})
	}
	for len(r.jobs) > keep && len(done) > 0 {
		oldest := 0
		for i := 1; i < len(done); i++ {
			if done[i].at.Before(done[oldest].at) {
				oldest = i
			}
		}
		delete(r.jobs, done[oldest].id)
		done = append(done[:oldest], done[oldest+1:]...)
	}
}

func (j *FixJob) Clone() *FixJob {
	if j == nil {
		return nil
	}
	cp := *j
	if j.FinishedAt != nil {
		t := *j.FinishedAt
		cp.FinishedAt = &t
	}
	if j.Fixes != nil {
		cp.Fixes = append([]Fix(nil), j.Fixes...)
	}
	if j.Results != nil {
		cp.Results = append([]FixResult(nil), j.Results...)
	}
	cp.Cancel = nil
	return &cp
}
