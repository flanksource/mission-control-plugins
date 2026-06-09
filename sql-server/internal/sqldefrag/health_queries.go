package sqldefrag

import (
	"fmt"
	"strings"
)

// The health queries read live DMVs and MUST run on a connection already
// scoped to the target database via `USE [db]` so OBJECT_ID/DB_ID() resolve
// correctly. Page counts are converted to bytes via page_count*8*1024 (8 KB
// pages) so UI formatting can render exact KB/MB/GB without integer MB loss.
func buildTableSizeQuery(table string, limit int) (string, []any) {
	q := `SELECT TOP (?) ps.object_id AS object_id,
  SCHEMA_NAME(o.schema_id) AS schema_name,
  OBJECT_NAME(ps.object_id) AS table_name,
  SUM(CASE WHEN ps.index_id < 2 THEN ps.row_count ELSE 0 END) AS table_rows,
  SUM(ps.reserved_page_count)*8*1024 AS total_bytes,
  SUM(CASE WHEN ps.index_id < 2
           THEN ps.in_row_data_page_count + ps.lob_used_page_count + ps.row_overflow_used_page_count
           ELSE ps.lob_used_page_count + ps.row_overflow_used_page_count END)*8*1024 AS data_bytes,
  (SUM(ps.used_page_count) - SUM(CASE WHEN ps.index_id < 2
           THEN ps.in_row_data_page_count + ps.lob_used_page_count + ps.row_overflow_used_page_count
           ELSE ps.lob_used_page_count + ps.row_overflow_used_page_count END))*8*1024 AS index_bytes,
  (SUM(ps.reserved_page_count) - SUM(ps.used_page_count))*8*1024 AS unused_bytes
FROM sys.dm_db_partition_stats ps
JOIN sys.objects o ON o.object_id = ps.object_id
WHERE o.type = 'U'`
	args := []any{limit}
	if table != "" {
		q += ` AND ps.object_id = OBJECT_ID(?)`
		args = append(args, objectIDArg(table))
	}
	q += `
GROUP BY ps.object_id, o.schema_id
ORDER BY total_bytes DESC`
	return q, args
}

func buildIndexFragQuery(table, scanMode string, objectIDs []int64) (string, []any) {
	object := "NULL"
	args := []any{}
	if table != "" {
		object = "OBJECT_ID(?)"
		args = append(args, objectIDArg(table))
	}
	// sz pre-aggregates reserved pages per (object,index) so the LEFT JOIN adds
	// exactly one row; joining sys.dm_db_partition_stats directly would fan out
	// against the per-level/per-partition rows ips returns and inflate the sum.
	// scanMode is whitelisted by validateScanMode before reaching here; it
	// cannot be a bound parameter inside the DMV call.
	// us is LEFT JOINed and scoped to DB_ID(): an index never read since the
	// last restart has no usage row, and must surface as zero counts.
	q := fmt.Sprintf(`SELECT ips.object_id AS object_id,
  i.index_id AS index_id,
  i.name AS index_name,
  i.type_desc AS type_desc,
  i.is_primary_key AS is_primary_key,
  i.is_unique AS is_unique,
  ips.avg_fragmentation_in_percent AS avg_fragmentation_in_percent,
  ips.page_count AS page_count,
  ips.record_count AS record_count,
  ISNULL(sz.reserved_pages, 0)*8*1024 AS index_bytes,
  ISNULL(us.user_seeks, 0) AS user_seeks,
  ISNULL(us.user_scans, 0) AS user_scans,
  ISNULL(us.user_lookups, 0) AS user_lookups,
  ISNULL(us.user_updates, 0) AS user_updates,
  (SELECT MAX(v) FROM (VALUES (us.last_user_seek), (us.last_user_scan), (us.last_user_lookup)) AS r(v)) AS last_read,
  us.last_user_update AS last_write
FROM sys.dm_db_index_physical_stats(DB_ID(), %s, NULL, NULL, '%s') ips
JOIN sys.indexes i ON i.object_id = ips.object_id AND i.index_id = ips.index_id
LEFT JOIN (
  SELECT object_id, index_id, SUM(reserved_page_count) AS reserved_pages
  FROM sys.dm_db_partition_stats
  GROUP BY object_id, index_id
) sz ON sz.object_id = ips.object_id AND sz.index_id = ips.index_id
LEFT JOIN sys.dm_db_index_usage_stats us
  ON us.object_id = ips.object_id AND us.index_id = ips.index_id AND us.database_id = DB_ID()
WHERE i.index_id > 0`, object, scanMode)
	if table == "" {
		q += inClause("ips.object_id", objectIDs, &args)
	}
	q += `
ORDER BY ips.object_id, ips.avg_fragmentation_in_percent DESC`
	return q, args
}

// buildIndexColumnsQuery reads the key and INCLUDE columns per index from
// sys.index_columns, ordered by key_ordinal so the caller rebuilds the key in
// definition order. Included columns have key_ordinal 0; sorting by
// is_included_column then key_ordinal keeps key columns ahead of them.
func buildIndexColumnsQuery(table string, objectIDs []int64) (string, []any) {
	q := `SELECT ic.object_id AS object_id,
  ic.index_id AS index_id,
  c.name AS column_name,
  ic.is_included_column AS is_included_column
FROM sys.index_columns ic
JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
JOIN sys.indexes i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
WHERE i.index_id > 0`
	args := []any{}
	if table != "" {
		q += ` AND ic.object_id = OBJECT_ID(?)`
		args = append(args, objectIDArg(table))
	} else {
		q += inClause("ic.object_id", objectIDs, &args)
	}
	q += `
ORDER BY ic.object_id, ic.index_id, ic.is_included_column, ic.key_ordinal`
	return q, args
}

func buildStatsHealthQuery(table string, objectIDs []int64) (string, []any) {
	q := `SELECT s.object_id AS object_id,
  s.name AS stat_name,
  c.name AS lead_column,
  sp.last_updated AS last_updated,
  sp.rows AS stat_rows,
  sp.rows_sampled AS rows_sampled,
  sp.modification_counter AS modification_counter,
  sp.steps AS steps,
  s.auto_created AS auto_created,
  s.no_recompute AS no_recompute
FROM sys.stats s
CROSS APPLY sys.dm_db_stats_properties(s.object_id, s.stats_id) sp
LEFT JOIN sys.stats_columns sc ON sc.object_id = s.object_id AND sc.stats_id = s.stats_id AND sc.stats_column_id = 1
LEFT JOIN sys.columns c ON c.object_id = sc.object_id AND c.column_id = sc.column_id
WHERE OBJECTPROPERTY(s.object_id, 'IsUserTable') = 1`
	args := []any{}
	if table != "" {
		q += ` AND s.object_id = OBJECT_ID(?)`
		args = append(args, objectIDArg(table))
	} else {
		q += inClause("s.object_id", objectIDs, &args)
	}
	q += `
ORDER BY s.object_id, sp.modification_counter DESC`
	return q, args
}

// objectIDArg keeps unqualified table input compatible with OIPA's dbo default
// while accepting schema-qualified names (dbo.Policy) from the UI.
func objectIDArg(table string) string {
	table = strings.TrimSpace(table)
	if table == "" || strings.Contains(table, ".") || strings.Contains(table, "[") {
		return table
	}
	return "dbo." + table
}

// inClause appends `AND <col> IN (?, ?, …)` and the bound args. With no ids it
// appends a contradiction so the query returns no rows rather than every row.
func inClause(col string, ids []int64, args *[]any) string {
	if len(ids) == 0 {
		return " AND 1 = 0"
	}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		*args = append(*args, id)
	}
	return " AND " + col + " IN (" + strings.Join(placeholders, ", ") + ")"
}
