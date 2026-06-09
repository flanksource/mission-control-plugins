package sqldefrag

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// fragReorganizeFloor is the fragmentation percentage at which a REORGANIZE is
// worth offering. Between it and fragBadPercent (30%) a REORGANIZE suffices;
// above fragBadPercent the index warrants a REBUILD.
const fragReorganizeFloor = 20.0

const (
	statsCollapseThreshold   = 2
	rebuildCollapseThreshold = 2
	statsLowSamplePct        = 25.0
	statsFullScanMinRows     = 1_000_000
)

// RebuildOptions controls the WITH (...) clause emitted for ALTER INDEX REBUILD.
type RebuildOptions struct {
	Offline   bool `json:"offline,omitempty"`
	Resumable bool `json:"resumable,omitempty"`
	MaxDop    int  `json:"maxDop,omitempty"`
}

type FixKind string

const (
	FixRebuild        FixKind = "REBUILD"
	FixRebuildAll     FixKind = "REBUILD ALL"
	FixReorganize     FixKind = "REORGANIZE"
	FixUpdateStats    FixKind = "UPDATE STATISTICS"
	FixUpdateAllStats FixKind = "UPDATE ALL STATS"
	FixDropIndex      FixKind = "DROP INDEX"
)

// SampleMode chooses how statistics are re-read on an UPDATE STATISTICS.
type SampleMode string

const (
	SampleAuto    SampleMode = "auto"
	SampleSampled SampleMode = "sampled"
	SampleFull    SampleMode = "full"
)

func ParseSampleMode(s string) (SampleMode, error) {
	switch SampleMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", SampleAuto:
		return SampleAuto, nil
	case SampleSampled:
		return SampleSampled, nil
	case SampleFull:
		return SampleFull, nil
	default:
		return "", fmt.Errorf("invalid statsSample %q: must be auto, sampled, or full", s)
	}
}

// Fix is one remediation against a single index or statistics object. SQL is
// built once at recommendation time with every identifier bracket-quoted, so
// callers execute it verbatim.
type Fix struct {
	Kind     FixKind    `json:"kind"`
	Schema   string     `json:"schema"`
	Table    string     `json:"table"`
	Target   string     `json:"target"`
	Detail   string     `json:"detail"`
	Sample   SampleMode `json:"sample,omitempty"`
	SQL      string     `json:"sql"`
	Rollback string     `json:"rollback,omitempty"`
}

// FixResult is the outcome of applying a single Fix.
type FixResult struct {
	Fix      Fix      `json:"fix"`
	Applied  bool     `json:"applied"`
	Error    string   `json:"error,omitempty"`
	Messages []string `json:"messages,omitempty"`
}

// RecommendFixes triages a HealthResult into remediations:
//   - REORGANIZE for fragmentation 20..30%, REBUILD above 30% (page floor applies)
//   - 2+ per-index rebuilds on a table collapse to ALTER INDEX ALL REBUILD
//   - stale stats collapse to table-wide UPDATE STATISTICS when there are 2+
//   - duplicate/unused non-essential indexes are surfaced as DROP INDEX fixes
func RecommendFixes(result HealthResult, rebuild RebuildOptions, sample SampleMode) []Fix {
	var fixes []Fix
	for _, tbl := range result.Tables {
		idxFixes, rebuilt, collapsed := indexFixesFor(tbl, rebuild)
		fixes = append(fixes, idxFixes...)
		if collapsed {
			continue
		}

		var stale []StatHealth
		for _, s := range tbl.Stats {
			if s.Stale && !rebuilt[s.Name] {
				stale = append(stale, s)
			}
		}
		fixes = append(fixes, statsFixesFor(tbl, stale, sample)...)
	}
	return orderFixes(fixes)
}

// TableRef identifies a table selected for a bulk maintenance run.
type TableRef struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
}

// BulkRebuildFixes builds whole-table maintenance fixes for operator-selected
// tables, independent of health triage.
func BulkRebuildFixes(tables []TableRef, rebuildIndexes, updateStats bool, rebuild RebuildOptions, sample SampleMode) []Fix {
	if len(tables) == 0 || (!rebuildIndexes && !updateStats) {
		return nil
	}
	var fixes []Fix
	for _, t := range tables {
		schema := strings.TrimSpace(t.Schema)
		table := strings.TrimSpace(t.Table)
		if schema == "" {
			schema = "dbo"
		}
		if table == "" {
			continue
		}
		switch {
		case rebuildIndexes:
			fixes = append(fixes, allRebuildFix(schema, table, rebuild))
		case updateStats:
			fixes = append(fixes, tableStatsFix(schema, table, sample))
		}
	}
	return orderFixes(fixes)
}

func indexFixesFor(tbl TableHealth, rebuild RebuildOptions) (fixes []Fix, rebuilt map[string]bool, collapsed bool) {
	rebuilt = make(map[string]bool)
	var maint []Fix
	var drops []Fix
	rebuildCount := 0
	for _, ix := range tbl.Indexes {
		if ix.DropCandidate {
			drops = append(drops, dropFix(tbl.Schema, tbl.TableName, ix))
			continue
		}
		if ix.PageCount <= fragMinPages || ix.Fragmentation < fragReorganizeFloor {
			continue
		}
		kind := FixReorganize
		if ix.Fragmentation > fragBadPercent {
			kind = FixRebuild
			rebuilt[ix.Name] = true
			rebuildCount++
		}
		maint = append(maint, indexFix(kind, tbl.Schema, tbl.TableName, ix, rebuild))
	}

	if rebuildCount >= rebuildCollapseThreshold {
		return append([]Fix{allRebuildFix(tbl.Schema, tbl.TableName, rebuild)}, drops...), nil, true
	}
	return append(maint, drops...), rebuilt, false
}

func statsFixesFor(tbl TableHealth, stale []StatHealth, sample SampleMode) []Fix {
	switch {
	case len(stale) == 0:
		return nil
	case len(stale) < statsCollapseThreshold:
		s := stale[0]
		return []Fix{statFix(tbl.Schema, tbl.TableName, s, resolveStatSample(sample, s, tbl.Rows))}
	default:
		mode := tableSample(sample)
		if sample == SampleAuto {
			for _, s := range stale {
				if resolveStatSample(sample, s, tbl.Rows) == SampleFull {
					mode = SampleFull
					break
				}
			}
		}
		return []Fix{tableStatsFix(tbl.Schema, tbl.TableName, mode)}
	}
}

func resolveStatSample(mode SampleMode, s StatHealth, rows int64) SampleMode {
	switch mode {
	case SampleSampled:
		return SampleSampled
	case SampleFull:
		return SampleFull
	default:
		if s.PctSampled < statsLowSamplePct && rows >= statsFullScanMinRows {
			return SampleFull
		}
		return SampleSampled
	}
}

func tableSample(mode SampleMode) SampleMode {
	if mode == SampleFull {
		return SampleFull
	}
	return SampleSampled
}

// enterpriseEngineEditions are SERVERPROPERTY('EngineEdition') values that
// support ONLINE/RESUMABLE index rebuilds: 3 = Enterprise/Developer,
// 5 = Azure SQL Database, 8 = Azure SQL Managed Instance.
var enterpriseEngineEditions = map[int]bool{3: true, 5: true, 8: true}

func supportsOnlineRebuild(ctx context.Context, db *gorm.DB) (bool, int, error) {
	var edition int
	if err := db.WithContext(ctx).Raw("SELECT CAST(SERVERPROPERTY('EngineEdition') AS int)").Scan(&edition).Error; err != nil {
		return false, 0, fmt.Errorf("detect engine edition: %w", err)
	}
	return enterpriseEngineEditions[edition], edition, nil
}

func rebuildClause(opts RebuildOptions) string {
	var parts []string
	if !opts.Offline {
		parts = append(parts, "ONLINE = ON")
		if opts.Resumable {
			parts = append(parts, "RESUMABLE = ON")
		}
	}
	parts = append(parts, "SORT_IN_TEMPDB = ON")
	if opts.MaxDop > 0 {
		parts = append(parts, fmt.Sprintf("MAXDOP = %d", opts.MaxDop))
	}
	return " WITH (" + strings.Join(parts, ", ") + ")"
}

func indexFix(kind FixKind, schema, table string, ix IndexHealth, rebuild RebuildOptions) Fix {
	verb, clause := "REORGANIZE", ""
	if kind == FixRebuild {
		verb, clause = "REBUILD", rebuildClause(rebuild)
	}
	sql := fmt.Sprintf("ALTER INDEX %s ON %s.%s %s%s;",
		bracketName(ix.Name), bracketName(schema), bracketName(table), verb, clause)
	return Fix{
		Kind:   kind,
		Schema: schema,
		Table:  table,
		Target: ix.Name,
		Detail: fmt.Sprintf("frag %.1f%%, %d pages", ix.Fragmentation, ix.PageCount),
		SQL:    sql,
	}
}

func allRebuildFix(schema, table string, rebuild RebuildOptions) Fix {
	sql := fmt.Sprintf("ALTER INDEX ALL ON %s.%s REBUILD%s;",
		bracketName(schema), bracketName(table), rebuildClause(rebuild))
	return Fix{
		Kind:   FixRebuildAll,
		Schema: schema,
		Table:  table,
		Target: "(all indexes)",
		Detail: "2+ indexes fragmented; rebuilds every index and refreshes index stats",
		SQL:    sql,
	}
}

func dropFix(schema, table string, ix IndexHealth) Fix {
	sql := fmt.Sprintf("DROP INDEX %s ON %s.%s;",
		bracketName(ix.Name), bracketName(schema), bracketName(table))
	return Fix{
		Kind:     FixDropIndex,
		Schema:   schema,
		Table:    table,
		Target:   ix.Name,
		Detail:   dropReason(ix),
		SQL:      sql,
		Rollback: createIndexDDL(schema, table, ix),
	}
}

func dropReason(ix IndexHealth) string {
	switch {
	case ix.Duplicate && ix.DuplicateOf != "":
		return fmt.Sprintf("redundant: key is a prefix of %s", ix.DuplicateOf)
	case ix.Duplicate:
		return "redundant: key is a prefix of a wider index"
	default:
		return fmt.Sprintf("unused: 0 reads, %d writes (verify usage counters are reliable)", ix.Updates)
	}
}

func createIndexDDL(schema, table string, ix IndexHealth) string {
	if len(ix.KeyColumns) == 0 {
		return ""
	}
	unique := ""
	if ix.IsUnique {
		unique = "UNIQUE "
	}
	create := fmt.Sprintf("CREATE %s%s INDEX %s ON %s.%s (%s)",
		unique, indexKind(ix.TypeDesc), bracketName(ix.Name), bracketName(schema), bracketName(table),
		strings.Join(bracketAll(ix.KeyColumns), ", "))
	if len(ix.IncludedColumns) > 0 {
		create += " INCLUDE (" + strings.Join(bracketAll(ix.IncludedColumns), ", ") + ")"
	}
	return create + ";"
}

func indexKind(typeDesc string) string {
	if strings.EqualFold(typeDesc, "CLUSTERED") {
		return "CLUSTERED"
	}
	return "NONCLUSTERED"
}

func bracketAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = bracketName(n)
	}
	return out
}

func fullScanSuffix(mode SampleMode) string {
	if mode == SampleFull {
		return " WITH FULLSCAN"
	}
	return ""
}

func statFix(schema, table string, s StatHealth, mode SampleMode) Fix {
	sql := fmt.Sprintf("UPDATE STATISTICS %s.%s (%s)%s;",
		bracketName(schema), bracketName(table), bracketName(s.Name), fullScanSuffix(mode))
	detail := fmt.Sprintf("changed %.1f%%, sampled %.1f%%, updated %s", s.PctChanged, s.PctSampled, fmtStatTime(s.LastUpdated))
	if mode == SampleFull {
		detail += ", fullscan"
	}
	return Fix{
		Kind:   FixUpdateStats,
		Schema: schema,
		Table:  table,
		Target: s.Name,
		Detail: detail,
		Sample: mode,
		SQL:    sql,
	}
}

func tableStatsFix(schema, table string, mode SampleMode) Fix {
	sql := fmt.Sprintf("UPDATE STATISTICS %s.%s%s;",
		bracketName(schema), bracketName(table), fullScanSuffix(mode))
	detail := "every statistic on the table"
	if mode == SampleFull {
		detail += ", fullscan"
	}
	return Fix{
		Kind:   FixUpdateAllStats,
		Schema: schema,
		Table:  table,
		Target: "(all stats)",
		Detail: detail,
		Sample: mode,
		SQL:    sql,
	}
}

func kindRank(k FixKind) int {
	switch k {
	case FixRebuild, FixRebuildAll, FixReorganize:
		return 0
	case FixDropIndex:
		return 2
	default:
		return 1
	}
}

func orderFixes(fixes []Fix) []Fix {
	ordered := make([]Fix, len(fixes))
	copy(ordered, fixes)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Schema != b.Schema {
			return a.Schema < b.Schema
		}
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		return kindRank(a.Kind) < kindRank(b.Kind)
	})
	return ordered
}

// ApplyFixes runs each Fix serially on its own borrowed connection. A failing
// fix is recorded and the run continues so one problematic index/statistic does
// not abort the rest of the batch.
func ApplyFixes(ctx context.Context, db *gorm.DB, database string, fixes []Fix) ([]FixResult, error) {
	resolved, err := resolveDatabase(ctx, db, database)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, fmt.Errorf("apply fixes requires a single database; 'all' is not supported")
	}
	ordered := orderFixes(fixes)
	results := make([]FixResult, 0, len(ordered))
	for _, f := range ordered {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		results = append(results, applyOneFix(ctx, db, resolved, f))
	}
	return results, nil
}

func applyOneFix(ctx context.Context, db *gorm.DB, database string, f Fix) FixResult {
	res := FixResult{Fix: f, Messages: []string{f.SQL}}
	err := db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		return tx.Exec(f.SQL).Error
	})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Applied = true
	return res
}
