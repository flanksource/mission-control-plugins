package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqldefrag"
)

type DefragInstallParams struct {
	Source              string `json:"source,omitempty"`
	MaintenanceDatabase string `json:"maintenanceDatabase,omitempty"`
}

type DefragFixParams struct {
	Database string          `json:"database,omitempty"`
	Fixes    []sqldefrag.Fix `json:"fixes"`
}

type DefragFixStopParams struct {
	ID string `json:"id,omitempty"`
}

type DefragRollbacksParams struct {
	Database string `json:"database,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type DefragRollbacksResponse struct {
	Database  string                 `json:"database"`
	Rollbacks []sqldefrag.AuditEntry `json:"rollbacks"`
}

type DefragRollbackRestoreParams struct {
	Database string `json:"database,omitempty"`
	ID       int64  `json:"id"`
}

func (p *SQLServerPlugin) defragHealth(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.HealthOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	if r.BoundDatabase != "" {
		opts.Database = r.BoundDatabase
	}
	if err := sqldefrag.ValidateHealthOptions(opts); err != nil {
		return nil, err
	}
	return sqldefrag.BuildHealthView(ctx, r.DB, opts)
}

func (p *SQLServerPlugin) defragFix(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params DefragFixParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	database := params.Database
	if r.BoundDatabase != "" {
		database = r.BoundDatabase
	}
	return p.fixJobs.StartWithDB(r.DB, database, params.Fixes)
}

func (p *SQLServerPlugin) defragBulkRebuild(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.BulkRebuildOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	if r.BoundDatabase != "" {
		opts.Database = r.BoundDatabase
	}
	return p.fixJobs.StartBulkWithDB(r.DB, opts)
}

func (p *SQLServerPlugin) defragFixJobsList(_ context.Context, _ sdk.InvokeCtx) (any, error) {
	return p.fixJobs.List(), nil
}

func (p *SQLServerPlugin) defragFixStop(_ context.Context, req sdk.InvokeCtx) (any, error) {
	var params DefragFixStopParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	if params.ID != "" {
		job, ok := p.fixJobs.Stop(params.ID)
		if !ok {
			return nil, nil
		}
		return job, nil
	}
	return p.fixJobs.StopRunning(), nil
}

func (p *SQLServerPlugin) defragRollbacks(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params DefragRollbacksParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	database := params.Database
	if r.BoundDatabase != "" {
		database = r.BoundDatabase
	}
	resolved, err := sqldefrag.ResolveDatabase(ctx, r.DB, database)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, fmt.Errorf("rollbacks require a single database; 'all' is not supported")
	}
	entries, err := sqldefrag.NewAuditLog(r.DB).List(ctx, resolved, sqldefrag.AuditDropIndex, params.Limit)
	if err != nil {
		return nil, err
	}
	return DefragRollbacksResponse{Database: resolved, Rollbacks: entries}, nil
}

func (p *SQLServerPlugin) defragRollbackRestore(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params DefragRollbackRestoreParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	database := params.Database
	if r.BoundDatabase != "" {
		database = r.BoundDatabase
	}
	return p.fixJobs.StartRollbackRestoreWithDB(r.DB, database, params.ID)
}

func (p *SQLServerPlugin) defragInstall(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params DefragInstallParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.Install(ctx, db, sqldefrag.InstallOptions{
		Source:              params.Source,
		MaintenanceDatabase: params.MaintenanceDatabase,
	})
}

func (p *SQLServerPlugin) defragRun(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.RunOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return p.defragJobs.StartWithDB(db, opts)
}

func (p *SQLServerPlugin) defragStatus(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.StatusOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.GetStatus(ctx, db, opts)
}

func (p *SQLServerPlugin) defragStats(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.StatsOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.Stats(ctx, db, opts)
}

func (p *SQLServerPlugin) defragHistory(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.HistoryOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.History(ctx, db, opts)
}

func (p *SQLServerPlugin) defragSessions(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.StopOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.ListRunningRuns(ctx, db, opts)
}

func (p *SQLServerPlugin) defragTerminate(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var opts sqldefrag.StopOptions
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &opts); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	db := r.DB
	return sqldefrag.TerminateExistingRuns(ctx, db, opts)
}

func (p *SQLServerPlugin) defragJobsList(_ context.Context, _ sdk.InvokeCtx) (any, error) {
	return p.defragJobs.List(), nil
}

func (p *SQLServerPlugin) defragStop(_ context.Context, _ sdk.InvokeCtx) (any, error) {
	return p.defragJobs.StopRunning(), nil
}
