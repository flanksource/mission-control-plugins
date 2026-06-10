// Package sqlperms probes the connected SQL Server login's effective
// permissions and reports, per diagnostics capability, which grants are missing
// and the GRANT statements that close the gap.
package sqlperms

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Category identifies one diagnostics capability whose SQL Server permission
// requirements are reported independently.
type Category string

const (
	CategoryMetrics    Category = "metrics"
	CategoryInspection Category = "inspection"
	CategoryHealthView Category = "healthView"
	CategoryHealthFix  Category = "healthFix"
	CategoryDefrag     Category = "defrag"
)

// CategoryResult is the per-capability verdict: whether the login can use it
// and, if not, the missing permission names plus copy-pasteable GRANT statements.
type CategoryResult struct {
	Category           Category `json:"category"`
	Label              string   `json:"label"`
	Granted            bool     `json:"granted"`
	MissingPermissions []string `json:"missingPermissions,omitempty"`
	GrantStatements    []string `json:"grantStatements,omitempty"`
	Note               string   `json:"note,omitempty"`
}

// Report is the full permission picture for one login against one connection.
type Report struct {
	Login               string           `json:"login"`
	MaintenanceDatabase string           `json:"maintenanceDatabase"`
	IsSysadmin          bool             `json:"isSysadmin"`
	EngineEdition       int              `json:"engineEdition"`
	Categories          []CategoryResult `json:"categories"`
	Warnings            []string         `json:"warnings,omitempty"`
}

func (r Report) AllGranted() bool {
	for _, c := range r.Categories {
		if !c.Granted {
			return false
		}
	}
	return true
}

type probe struct {
	login             string
	currentDatabase   string
	isSysadmin        bool
	viewServerState   bool
	viewAnyDefinition bool
	viewDatabaseState bool
	alterCurrentDB    bool
	createProcedure   bool
	createTable       bool
	alterMaintDB      bool
	isMaintDBOwner    bool
	engineEdition     int
	warnings          []string
}

type serverProbeRow struct {
	IsSysadmin        *int
	ViewServerState   *int
	ViewAnyDefinition *int
	ViewDatabaseState *int
	AlterCurrentDB    *int
	EngineEdition     int
	Login             string
	CurrentDatabase   string
}

type maintProbeRow struct {
	CreateProcedure *int
	CreateTable     *int
	AlterDB         *int
	ControlDB       *int
}

// CheckPermissions probes the connected login's capabilities across all SQL
// Server plugin categories. A connection failure returns an error; a maintenance
// DB probe failure is recorded as a warning and treated as not-granted.
func CheckPermissions(ctx context.Context, db *gorm.DB, maintenanceDatabase string) (Report, error) {
	p := probe{}

	var srv serverProbeRow
	if err := db.WithContext(ctx).Raw(serverProbeSQL).Scan(&srv).Error; err != nil {
		return Report{}, fmt.Errorf("probe server permissions: %w", err)
	}
	applyServerProbe(&p, srv)

	if maintenanceDatabase != "" {
		probeMaintenanceDB(ctx, db, maintenanceDatabase, &p)
	}

	return BuildReport(p, maintenanceDatabase), nil
}

const serverProbeSQL = `
SELECT
  IS_SRVROLEMEMBER('sysadmin')                                       AS is_sysadmin,
  HAS_PERMS_BY_NAME(NULL, NULL, 'VIEW SERVER STATE')                 AS view_server_state,
  HAS_PERMS_BY_NAME(NULL, NULL, 'VIEW ANY DEFINITION')               AS view_any_definition,
  HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', 'VIEW DATABASE STATE')    AS view_database_state,
  HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', 'ALTER')                  AS alter_current_db,
  CAST(SERVERPROPERTY('EngineEdition') AS int)                       AS engine_edition,
  SUSER_SNAME()                                                      AS login,
  DB_NAME()                                                          AS current_database`

const maintProbeSQL = `
SELECT
  HAS_PERMS_BY_NAME(?, 'DATABASE', 'CREATE PROCEDURE') AS create_procedure,
  HAS_PERMS_BY_NAME(?, 'DATABASE', 'CREATE TABLE')     AS create_table,
  HAS_PERMS_BY_NAME(?, 'DATABASE', 'ALTER')            AS alter_db,
  HAS_PERMS_BY_NAME(?, 'DATABASE', 'CONTROL')          AS control_db`

func applyServerProbe(p *probe, row serverProbeRow) {
	p.login = row.Login
	p.currentDatabase = row.CurrentDatabase
	p.engineEdition = row.EngineEdition
	p.isSysadmin = isTrue(row.IsSysadmin)
	p.viewServerState = isTrue(row.ViewServerState)
	p.viewAnyDefinition = isTrue(row.ViewAnyDefinition)
	p.viewDatabaseState = isTrue(row.ViewDatabaseState)
	p.alterCurrentDB = isTrue(row.AlterCurrentDB)
}

// probeMaintenanceDB checks database-scoped permissions by name instead of
// issuing USE, so this operation does not mutate even the pooled session state.
func probeMaintenanceDB(ctx context.Context, db *gorm.DB, maintenanceDatabase string, p *probe) {
	var row maintProbeRow
	err := db.WithContext(ctx).Raw(
		maintProbeSQL,
		maintenanceDatabase,
		maintenanceDatabase,
		maintenanceDatabase,
		maintenanceDatabase,
	).Scan(&row).Error
	if err != nil {
		p.warnings = append(p.warnings, fmt.Sprintf(
			"could not probe defrag permissions on %s: %v", maintenanceDatabase, err))
		return
	}
	p.createProcedure = isTrue(row.CreateProcedure)
	p.createTable = isTrue(row.CreateTable)
	p.alterMaintDB = isTrue(row.AlterDB)
	// db_owner implies CONTROL at database scope; keeping the field name preserves
	// the report mapping's role shortcut without switching database context.
	p.isMaintDBOwner = isTrue(row.ControlDB)
}

func isTrue(v *int) bool { return v != nil && *v == 1 }

func bracketName(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}
