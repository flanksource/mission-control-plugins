package sqldefrag

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// statsStaleDays is the age beyond which a statistics object is considered
// stale even if its modification backlog is low.
const statsStaleDays = 7

// fragBadPercent / fragMinPages bound when fragmentation is worth flagging:
// small indexes fragment heavily but cost little to scan or rebuild.
const (
	fragBadPercent  = 30.0
	fragMinPages    = 1000
	statsChangedPct = 20.0
)

var validScanModes = map[string]bool{"LIMITED": true, "SAMPLED": true, "DETAILED": true}

type HealthOptions struct {
	Database string `json:"database,omitempty"`
	Table    string `json:"table,omitempty"`
	ScanMode string `json:"scanMode,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type HealthResult struct {
	Database string        `json:"database"`
	ScanMode string        `json:"scanMode"`
	Table    string        `json:"table,omitempty"`
	Tables   []TableHealth `json:"tables"`
}

// HealthView is the plugin payload for defrag-health: the scan result plus the
// fixes RecommendFixes triages from it and server capability context for the UI.
// ProductMajorVersion and UsageReliable let the UI caveat per-index usage
// counts: sys.dm_db_index_usage_stats resets on restart and, before SQL 2019,
// can reset on index rebuild.
type HealthView struct {
	Health              HealthResult `json:"health"`
	Fixes               []Fix        `json:"fixes"`
	EngineEdition       int          `json:"engineEdition"`
	OnlineRebuild       bool         `json:"onlineRebuild"`
	ProductMajorVersion int          `json:"productMajorVersion"`
	UptimeDays          int          `json:"uptimeDays"`
	UsageReliable       bool         `json:"usageReliable"`
	Warnings            []string     `json:"warnings,omitempty"`
}

const (
	usageReliableMinVersion    = 15
	usageReliableMinUptimeDays = 7
)

// BuildHealthView runs a health scan and prepares recommended fixes using the
// live engine edition (forcing offline rebuilds where ONLINE is unsupported).
func BuildHealthView(ctx context.Context, db *gorm.DB, opts HealthOptions) (HealthView, error) {
	result, err := Health(ctx, db, opts)
	if err != nil {
		return HealthView{}, err
	}
	online, edition, err := supportsOnlineRebuild(ctx, db)
	if err != nil {
		return HealthView{}, err
	}
	major, uptimeDays, err := serverCapabilities(ctx, db)
	warnings := []string(nil)
	if err != nil {
		// The core health scan only needs database-level DMV visibility. Server
		// uptime is used to caveat unused-index counters, so degrade to
		// usageReliable=false instead of failing the whole scan for restricted
		// logins that lack VIEW SERVER STATE.
		warnings = append(warnings, err.Error())
	}
	rebuild := RebuildOptions{Offline: !online}
	fixes := RecommendFixes(result, rebuild, SampleAuto)
	return HealthView{
		Health:              result,
		Fixes:               fixes,
		EngineEdition:       edition,
		OnlineRebuild:       online,
		ProductMajorVersion: major,
		UptimeDays:          uptimeDays,
		UsageReliable:       major >= usageReliableMinVersion && uptimeDays >= usageReliableMinUptimeDays,
		Warnings:            warnings,
	}, nil
}

// serverCapabilities reads the product major version and uptime in days.
func serverCapabilities(ctx context.Context, db *gorm.DB) (majorVersion, uptimeDays int, err error) {
	var row struct {
		Major  int        `gorm:"column:major_version"`
		Uptime *time.Time `gorm:"column:sqlserver_start_time"`
	}
	q := `SELECT
  CONVERT(int, SERVERPROPERTY('ProductMajorVersion')) AS major_version,
  sqlserver_start_time
FROM sys.dm_os_sys_info`
	if err := db.WithContext(ctx).Raw(q).Scan(&row).Error; err != nil {
		return 0, 0, fmt.Errorf("detect server capabilities: %w", err)
	}
	if row.Uptime != nil {
		uptimeDays = int(time.Since(*row.Uptime).Hours() / 24)
	}
	return row.Major, uptimeDays, nil
}

type TableHealth struct {
	ObjectID     int64         `json:"-" gorm:"column:object_id"`
	Schema       string        `json:"schema" gorm:"column:schema_name"`
	TableName    string        `json:"tableName" gorm:"column:table_name"`
	Rows         int64         `json:"rows" gorm:"column:table_rows"`
	TotalBytes   int64         `json:"totalBytes" gorm:"column:total_bytes"`
	DataBytes    int64         `json:"dataBytes" gorm:"column:data_bytes"`
	IndexBytes   int64         `json:"indexBytes" gorm:"column:index_bytes"`
	UnusedBytes  int64         `json:"unusedBytes" gorm:"column:unused_bytes"`
	MaxFrag      float64       `json:"maxFragmentation" gorm:"-"`
	FragHealthy  bool          `json:"fragHealthy" gorm:"-"`
	StatsHealthy bool          `json:"statsHealthy" gorm:"-"`
	Indexes      []IndexHealth `json:"indexes" gorm:"-"`
	Stats        []StatHealth  `json:"statistics" gorm:"-"`
}

type IndexHealth struct {
	ObjectID      int64   `json:"-" gorm:"column:object_id"`
	IndexID       int64   `json:"-" gorm:"column:index_id"`
	Name          string  `json:"name" gorm:"column:index_name"`
	TypeDesc      string  `json:"type" gorm:"column:type_desc"`
	Fragmentation float64 `json:"fragmentation" gorm:"column:avg_fragmentation_in_percent"`
	PageCount     int64   `json:"pageCount" gorm:"column:page_count"`
	RecordCount   int64   `json:"recordCount" gorm:"column:record_count"`
	Bytes         int64   `json:"bytes" gorm:"column:index_bytes"`
	IsPrimaryKey  bool    `json:"isPrimaryKey" gorm:"column:is_primary_key"`
	IsUnique      bool    `json:"isUnique" gorm:"column:is_unique"`
	Bad           bool    `json:"bad" gorm:"-"`

	// Usage counters from sys.dm_db_index_usage_stats (since last restart, and
	// on SQL < 2019 since the last rebuild). Reads = seeks+scans+lookups.
	Seeks     int64      `json:"seeks" gorm:"column:user_seeks"`
	Scans     int64      `json:"scans" gorm:"column:user_scans"`
	Lookups   int64      `json:"lookups" gorm:"column:user_lookups"`
	Updates   int64      `json:"updates" gorm:"column:user_updates"`
	LastRead  *time.Time `json:"lastRead,omitempty" gorm:"column:last_read"`
	LastWrite *time.Time `json:"lastWrite,omitempty" gorm:"column:last_write"`

	KeyColumns      []string `json:"keyColumns" gorm:"-"`
	IncludedColumns []string `json:"includedColumns" gorm:"-"`

	// Duplicate is set when this index's leading key columns are a prefix of
	// another non-clustered index on the same table. Unused is set when reads are
	// zero; HealthView.UsageReliable conveys whether that zero is trustworthy.
	Duplicate     bool   `json:"duplicate" gorm:"-"`
	DuplicateOf   string `json:"duplicateOf,omitempty" gorm:"-"`
	Unused        bool   `json:"unused" gorm:"-"`
	DropCandidate bool   `json:"dropCandidate" gorm:"-"`
}

func (ix IndexHealth) reads() int64 { return ix.Seeks + ix.Scans + ix.Lookups }

type StatHealth struct {
	ObjectID            int64      `json:"-" gorm:"column:object_id"`
	Name                string     `json:"name" gorm:"column:stat_name"`
	LeadColumn          string     `json:"leadColumn,omitempty" gorm:"column:lead_column"`
	LastUpdated         *time.Time `json:"lastUpdated,omitempty" gorm:"column:last_updated"`
	Rows                int64      `json:"rows" gorm:"column:stat_rows"`
	RowsSampled         int64      `json:"rowsSampled" gorm:"column:rows_sampled"`
	ModificationCounter int64      `json:"modifications" gorm:"column:modification_counter"`
	Steps               int        `json:"steps" gorm:"column:steps"`
	AutoCreated         bool       `json:"autoCreated" gorm:"column:auto_created"`
	NoRecompute         bool       `json:"noRecompute" gorm:"column:no_recompute"`
	PctSampled          float64    `json:"pctSampled" gorm:"-"`
	PctChanged          float64    `json:"pctChanged" gorm:"-"`
	Stale               bool       `json:"stale" gorm:"-"`
}

// indexColumn is one (object,index) → column row from sys.index_columns,
// ordered so stitchHealth can rebuild the key and INCLUDE lists per index.
type indexColumn struct {
	ObjectID   int64  `gorm:"column:object_id"`
	IndexID    int64  `gorm:"column:index_id"`
	ColumnName string `gorm:"column:column_name"`
	IsIncluded bool   `gorm:"column:is_included_column"`
}

// validateScanMode normalizes and whitelists a fragmentation scan mode. The
// mode is interpolated into the DMV call, so it MUST be validated first.
func validateScanMode(mode string) (string, error) {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "" {
		mode = "LIMITED"
	}
	if !validScanModes[mode] {
		return "", fmt.Errorf("invalid scan-mode %q: must be LIMITED, SAMPLED, or DETAILED", mode)
	}
	return mode, nil
}

// ValidateHealthOptions checks scan-mode/table constraints without touching the
// database, so operation handlers can fail bad requests before the live scan.
func ValidateHealthOptions(opts HealthOptions) error {
	_, _, _, err := normalizeHealthOptions(opts)
	return err
}

func normalizeHealthOptions(opts HealthOptions) (scanMode, table string, limit int, err error) {
	scanMode, err = validateScanMode(opts.ScanMode)
	if err != nil {
		return "", "", 0, err
	}
	table = strings.TrimSpace(opts.Table)
	if scanMode == "DETAILED" && table == "" {
		return "", "", 0, fmt.Errorf("DETAILED scan requires table; use LIMITED or SAMPLED for whole-database health")
	}
	limit = opts.Limit
	if limit <= 0 {
		limit = 20
	}
	return scanMode, table, limit, nil
}

func Health(ctx context.Context, db *gorm.DB, opts HealthOptions) (HealthResult, error) {
	database, err := resolveDatabase(ctx, db, opts.Database)
	if err != nil {
		return HealthResult{}, err
	}
	if database == "" {
		return HealthResult{}, fmt.Errorf("health requires a single database; 'all' is not supported")
	}
	scanMode, table, limit, err := normalizeHealthOptions(opts)
	if err != nil {
		return HealthResult{}, err
	}

	result := HealthResult{Database: database, ScanMode: scanMode, Table: table}
	err = db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}

		tableQ, tableArgs := buildTableSizeQuery(table, limit)
		var tables []TableHealth
		if err := tx.Raw(tableQ, tableArgs...).Scan(&tables).Error; err != nil {
			return fmt.Errorf("query table sizes: %w", err)
		}
		if len(tables) == 0 {
			return nil
		}

		ids := make([]int64, len(tables))
		for i := range tables {
			ids[i] = tables[i].ObjectID
		}

		indexQ, indexArgs := buildIndexFragQuery(table, scanMode, ids)
		var indexes []IndexHealth
		if err := tx.Raw(indexQ, indexArgs...).Scan(&indexes).Error; err != nil {
			return fmt.Errorf("query index fragmentation: %w", err)
		}

		colQ, colArgs := buildIndexColumnsQuery(table, ids)
		var columns []indexColumn
		if err := tx.Raw(colQ, colArgs...).Scan(&columns).Error; err != nil {
			return fmt.Errorf("query index columns: %w", err)
		}

		statQ, statArgs := buildStatsHealthQuery(table, ids)
		var stats []StatHealth
		if err := tx.Raw(statQ, statArgs...).Scan(&stats).Error; err != nil {
			return fmt.Errorf("query statistics health: %w", err)
		}

		result.Tables = stitchHealth(tables, indexes, stats, columns)
		return nil
	})
	if err != nil {
		return HealthResult{}, err
	}
	return result, nil
}

type indexKey struct {
	objectID int64
	indexID  int64
}

func groupIndexColumns(columns []indexColumn) (key, included map[indexKey][]string) {
	key = map[indexKey][]string{}
	included = map[indexKey][]string{}
	for _, c := range columns {
		k := indexKey{c.ObjectID, c.IndexID}
		if c.IsIncluded {
			included[k] = append(included[k], c.ColumnName)
		} else {
			key[k] = append(key[k], c.ColumnName)
		}
	}
	return key, included
}

// flagIndexDropCandidates marks redundant and unused indexes within one table.
func flagIndexDropCandidates(indexes []IndexHealth) {
	for i := range indexes {
		ix := &indexes[i]
		ix.Unused = ix.reads() == 0
		if !droppable(*ix) {
			continue
		}
		for j := range indexes {
			if i == j {
				continue
			}
			if isKeyPrefix(ix.KeyColumns, indexes[j].KeyColumns) {
				ix.Duplicate = true
				ix.DuplicateOf = indexes[j].Name
				break
			}
		}
		ix.DropCandidate = ix.Duplicate || ix.Unused
	}
}

func droppable(ix IndexHealth) bool {
	return ix.IndexID > 1 && !ix.IsPrimaryKey && !ix.IsUnique && len(ix.KeyColumns) > 0
}

func isKeyPrefix(prefix, full []string) bool {
	if len(prefix) == 0 || len(prefix) > len(full) || len(prefix) == len(full) {
		return false
	}
	for i := range prefix {
		if !strings.EqualFold(prefix[i], full[i]) {
			return false
		}
	}
	return true
}

// stitchHealth groups indexes and stats under their table by object_id and
// computes the derived health flags.
func stitchHealth(tables []TableHealth, indexes []IndexHealth, stats []StatHealth, columns []indexColumn) []TableHealth {
	byID := make(map[int64]*TableHealth, len(tables))
	for i := range tables {
		tables[i].FragHealthy = true
		tables[i].StatsHealthy = true
		byID[tables[i].ObjectID] = &tables[i]
	}
	keyCols, inclCols := groupIndexColumns(columns)
	for i := range indexes {
		ix := &indexes[i]
		key := indexKey{ix.ObjectID, ix.IndexID}
		ix.KeyColumns = keyCols[key]
		ix.IncludedColumns = inclCols[key]
	}
	for _, ix := range indexes {
		t, ok := byID[ix.ObjectID]
		if !ok {
			continue
		}
		ix.Bad = ix.Fragmentation > fragBadPercent && ix.PageCount > fragMinPages
		if ix.Fragmentation > t.MaxFrag {
			t.MaxFrag = ix.Fragmentation
		}
		if ix.Bad {
			t.FragHealthy = false
		}
		t.Indexes = append(t.Indexes, ix)
	}
	for i := range tables {
		flagIndexDropCandidates(tables[i].Indexes)
	}
	for _, s := range stats {
		t, ok := byID[s.ObjectID]
		if !ok {
			continue
		}
		if s.Rows > 0 {
			s.PctSampled = 100.0 * float64(s.RowsSampled) / float64(s.Rows)
			s.PctChanged = 100.0 * float64(s.ModificationCounter) / float64(s.Rows)
		}
		s.Stale = s.PctChanged > statsChangedPct ||
			(s.LastUpdated != nil && time.Since(*s.LastUpdated) > statsStaleDays*24*time.Hour)
		if s.Stale {
			t.StatsHealthy = false
		}
		t.Stats = append(t.Stats, s)
	}
	sort.SliceStable(tables, func(i, j int) bool { return tables[i].TotalBytes > tables[j].TotalBytes })
	return tables
}

func fmtStatTime(ts *time.Time) string {
	if ts == nil {
		return "never"
	}
	return ts.Format("2006-01-02")
}
