package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models/uir"
	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqlinspect"
	"gorm.io/gorm"
)

// InspectParams is the input shape for the `inspect` operation. Database
// optionally selects the database to inspect; when the config item is itself an
// MSSQL::Database, the bound database wins. Refresh bypasses the 24h in-memory
// schema cache and re-runs the arch-unit SQL extractor.
type InspectParams struct {
	Database string `json:"database,omitempty"`
	Refresh  bool   `json:"refresh,omitempty"`
}

func (p *SQLServerPlugin) inspect(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params InspectParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	database, err := inspectDatabaseName(ctx, r.DB, r.BoundDatabase, params.Database)
	if err != nil {
		return nil, err
	}
	cacheKey := sqlinspect.CacheKey(req.ConfigItemID, database)
	requestedDatabase := strings.TrimSpace(params.Database) != ""
	return sqlinspect.DefaultCache.Load(cacheKey, params.Refresh, func() (uir.UIR, error) {
		db, cleanup, err := inspectDB(ctx, r, database, requestedDatabase)
		if err != nil {
			return uir.UIR{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
		return sqlinspect.Extract(db)
	})
}

func inspectDatabaseName(ctx context.Context, db *gorm.DB, boundDatabase, requestedDatabase string) (string, error) {
	if boundDatabase != "" {
		return boundDatabase, nil
	}
	requestedDatabase = strings.TrimSpace(requestedDatabase)
	if requestedDatabase != "" {
		return requestedDatabase, nil
	}
	if db == nil {
		return "", fmt.Errorf("resolve database name: nil db")
	}
	var database string
	if err := db.WithContext(ctx).Raw("SELECT DB_NAME()").Scan(&database).Error; err != nil {
		return "", fmt.Errorf("resolve database name: %w", err)
	}
	database = strings.TrimSpace(database)
	if database == "" {
		return "", fmt.Errorf("resolve database name: DB_NAME() returned empty")
	}
	return database, nil
}

func inspectDB(ctx context.Context, r *resolved, database string, requestedDatabase bool) (*gorm.DB, func(), error) {
	if r == nil || r.DB == nil {
		return nil, nil, fmt.Errorf("resolve inspect database: nil connection")
	}
	if r.BoundDatabase != "" || !requestedDatabase || database == "" {
		return r.DB, nil, nil
	}
	url, err := withDefaultDatabase(r.ConnType, r.ConnURL, database)
	if err != nil {
		return nil, nil, fmt.Errorf("scope inspect connection to database %q: %w", database, err)
	}
	db, err := openGorm(ctx, r.ConnType, url)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return db, cleanup, nil
}
