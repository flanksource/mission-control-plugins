package sqldefrag

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// auditLogTable is the per-database audit table used to persist rollback SQL
// for destructive health fixes. It lives in the target database so the saved
// CREATE INDEX statement is stored next to the index it can restore.
const auditLogTable = "MCAuditLog"

type AuditAction string

const (
	AuditDropIndex    AuditAction = "DROP INDEX"
	AuditRestoreIndex AuditAction = "RESTORE INDEX"
)

// AuditEntry mirrors one row in dbo.MCAuditLog.
type AuditEntry struct {
	ID          int64       `gorm:"column:AuditID;primaryKey;autoIncrement" json:"id"`
	CreatedAt   time.Time   `gorm:"column:CreatedAt" json:"createdAt"`
	Database    string      `gorm:"column:DatabaseName" json:"database"`
	SchemaName  string      `gorm:"column:SchemaName" json:"schema"`
	Table       string      `gorm:"column:TableName" json:"table"`
	ObjectName  string      `gorm:"column:ObjectName" json:"objectName"`
	Action      AuditAction `gorm:"column:Action" json:"action"`
	Reason      string      `gorm:"column:Reason" json:"reason"`
	AppliedSQL  string      `gorm:"column:AppliedSQL" json:"appliedSql"`
	RollbackSQL string      `gorm:"column:RollbackSQL" json:"rollbackSql"`
	RestoredAt  *time.Time  `gorm:"column:RestoredAt" json:"restoredAt,omitempty"`
}

type AuditLog struct {
	db *gorm.DB
}

func NewAuditLog(db *gorm.DB) *AuditLog {
	return &AuditLog{db: db}
}

func createAuditLogTableSQL() string {
	return fmt.Sprintf(`
IF NOT EXISTS (SELECT 1 FROM sys.tables t JOIN sys.schemas s ON s.schema_id = t.schema_id
               WHERE s.name = N'dbo' AND t.name = N'%s')
CREATE TABLE dbo.%s (
	AuditID      bigint IDENTITY(1,1) NOT NULL CONSTRAINT PK_%s PRIMARY KEY,
	CreatedAt    datetime2 NOT NULL CONSTRAINT DF_%s_CreatedAt DEFAULT SYSUTCDATETIME(),
	DatabaseName nvarchar(128) NOT NULL,
	SchemaName   nvarchar(128) NOT NULL,
	TableName    nvarchar(128) NOT NULL,
	ObjectName   nvarchar(256) NOT NULL,
	Action       nvarchar(64) NOT NULL,
	Reason       nvarchar(512) NULL,
	AppliedSQL   nvarchar(max) NULL,
	RollbackSQL  nvarchar(max) NULL,
	RestoredAt   datetime2 NULL
);`, auditLogTable, auditLogTable, auditLogTable, auditLogTable)
}

func (a *AuditLog) ensure(tx *gorm.DB) error {
	if err := tx.Exec(createAuditLogTableSQL()).Error; err != nil {
		return fmt.Errorf("ensure %s: %w", auditLogTable, err)
	}
	return nil
}

func (a *AuditLog) EnsureDatabase(ctx context.Context, database string) error {
	if database == "" {
		return fmt.Errorf("audit log: database is required")
	}
	if a == nil || a.db == nil {
		return fmt.Errorf("audit log: nil database connection")
	}
	return a.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		return a.ensure(tx)
	})
}

func (a *AuditLog) Append(ctx context.Context, database string, e AuditEntry) error {
	if database == "" {
		return fmt.Errorf("audit log: database is required")
	}
	if a == nil || a.db == nil {
		return fmt.Errorf("audit log: nil database connection")
	}
	return a.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		if err := a.ensure(tx); err != nil {
			return err
		}
		return tx.Exec(
			fmt.Sprintf(`INSERT INTO dbo.%s (DatabaseName, SchemaName, TableName, ObjectName, Action, Reason, AppliedSQL, RollbackSQL)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, auditLogTable),
			database, e.SchemaName, e.Table, e.ObjectName, string(e.Action), e.Reason, e.AppliedSQL, e.RollbackSQL,
		).Error
	})
}

func (a *AuditLog) List(ctx context.Context, database string, action AuditAction, limit int) ([]AuditEntry, error) {
	if database == "" {
		return nil, fmt.Errorf("audit log: database is required")
	}
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("audit log: nil database connection")
	}
	if limit > 500 {
		limit = 500
	}
	var entries []AuditEntry
	err := a.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		if err := a.ensure(tx); err != nil {
			return err
		}
		top := ""
		if limit > 0 {
			top = fmt.Sprintf("TOP (%d) ", limit)
		}
		query := fmt.Sprintf("SELECT %s* FROM dbo.%s", top, auditLogTable)
		args := []any{}
		if action != "" {
			query += " WHERE Action = ?"
			args = append(args, string(action))
		}
		query += " ORDER BY AuditID DESC"
		return tx.Raw(query, args...).Scan(&entries).Error
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (a *AuditLog) Get(ctx context.Context, database string, id int64) (AuditEntry, error) {
	if database == "" {
		return AuditEntry{}, fmt.Errorf("audit log: database is required")
	}
	if a == nil || a.db == nil {
		return AuditEntry{}, fmt.Errorf("audit log: nil database connection")
	}
	var entry AuditEntry
	err := a.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		if err := a.ensure(tx); err != nil {
			return err
		}
		return tx.Raw(fmt.Sprintf("SELECT * FROM dbo.%s WHERE AuditID = ?", auditLogTable), id).Scan(&entry).Error
	})
	if err != nil {
		return AuditEntry{}, err
	}
	if entry.ID == 0 {
		return AuditEntry{}, fmt.Errorf("audit entry %d not found in %s", id, database)
	}
	return entry, nil
}

func (a *AuditLog) MarkRestored(ctx context.Context, database string, id int64) error {
	if database == "" {
		return fmt.Errorf("audit log: database is required")
	}
	if a == nil || a.db == nil {
		return fmt.Errorf("audit log: nil database connection")
	}
	return a.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("USE " + bracketName(database)).Error; err != nil {
			return fmt.Errorf("use %s: %w", database, err)
		}
		if err := a.ensure(tx); err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("UPDATE dbo.%s SET RestoredAt = SYSUTCDATETIME() WHERE AuditID = ?", auditLogTable), id).Error
	})
}

func (a *AuditLog) RecordDrop(ctx context.Context, database string, f Fix) error {
	return a.Append(ctx, database, AuditEntry{
		SchemaName:  f.Schema,
		Table:       f.Table,
		ObjectName:  f.Target,
		Action:      AuditDropIndex,
		Reason:      f.Detail,
		AppliedSQL:  f.SQL,
		RollbackSQL: f.Rollback,
	})
}

func (a *AuditLog) RecordRestore(ctx context.Context, database string, src AuditEntry) error {
	return a.Append(ctx, database, AuditEntry{
		SchemaName: src.SchemaName,
		Table:      src.Table,
		ObjectName: src.ObjectName,
		Action:     AuditRestoreIndex,
		Reason:     fmt.Sprintf("restore of dropped index (audit #%d)", src.ID),
		AppliedSQL: src.RollbackSQL,
	})
}
