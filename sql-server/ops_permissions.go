package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/flanksource/incident-commander/plugin/sdk"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqldefrag"
	"github.com/flanksource/mission-control-plugins/sql-server/internal/sqlperms"
)

type PermissionsParams struct {
	MaintenanceDatabase string `json:"maintenanceDatabase,omitempty"`
}

func (p *SQLServerPlugin) permissions(ctx context.Context, req sdk.InvokeCtx) (any, error) {
	var params PermissionsParams
	if len(req.ParamsJSON) > 0 {
		if err := json.Unmarshal(req.ParamsJSON, &params); err != nil {
			return nil, err
		}
	}
	r, err := p.clients.For(ctx, req.Host, req.ConfigItemID)
	if err != nil {
		return nil, err
	}
	maintenanceDB := strings.TrimSpace(params.MaintenanceDatabase)
	if maintenanceDB == "" {
		maintenanceDB = sqldefrag.DefaultMaintenanceDatabase
	}
	return sqlperms.CheckPermissions(ctx, r.DB, maintenanceDB)
}
