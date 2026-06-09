package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqldefrag"
)

type RollbackListParams struct {
	Database string `json:"database,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type RollbackListResponse struct {
	Database  string                 `json:"database"`
	Rollbacks []sqldefrag.AuditEntry `json:"rollbacks"`
}

type RollbackRestoreParams struct {
	Database string `json:"database,omitempty"`
	ID       int64  `json:"id"`
}

func (p *SQLServerPlugin) rollbackList(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params RollbackListParams
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
	return RollbackListResponse{Database: resolved, Rollbacks: entries}, nil
}

func (p *SQLServerPlugin) rollbackRestore(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params RollbackRestoreParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	if params.ID <= 0 {
		return nil, fmt.Errorf("id must be a positive audit entry id")
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

// Deprecated defrag-* operation names are kept as aliases for existing links.
func (p *SQLServerPlugin) defragRollbacks(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	return p.rollbackList(ctx, req)
}

func (p *SQLServerPlugin) defragRollbackRestore(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	return p.rollbackRestore(ctx, req)
}
