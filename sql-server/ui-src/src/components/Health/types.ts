export interface PermissionReport {
  login: string;
  maintenanceDatabase: string;
  isSysadmin: boolean;
  engineEdition: number;
  categories: PermissionCategory[];
  warnings?: string[];
}

export type PermissionCategoryName =
  | "metrics"
  | "inspection"
  | "healthView"
  | "healthFix"
  | "defrag";

export interface PermissionCategory {
  category: PermissionCategoryName;
  label: string;
  granted: boolean;
  missingPermissions?: string[];
  grantStatements?: string[];
  note?: string;
}

export interface HealthView {
  health: HealthResult;
  fixes?: Fix[];
  engineEdition: number;
  onlineRebuild: boolean;
  productMajorVersion: number;
  uptimeDays: number;
  usageReliable: boolean;
  warnings?: string[];
}

export interface HealthResult {
  database: string;
  scanMode: string;
  table?: string;
  tables?: TableHealth[];
}

export interface TableHealth {
  schema: string;
  tableName: string;
  rows: number;
  totalBytes: number;
  dataBytes: number;
  indexBytes: number;
  unusedBytes: number;
  maxFragmentation: number;
  fragHealthy: boolean;
  statsHealthy: boolean;
  indexes?: IndexHealth[];
  statistics?: StatHealth[];
}

export interface IndexHealth {
  name: string;
  type: string;
  fragmentation: number;
  pageCount: number;
  bytes: number;
  bad: boolean;
  seeks: number;
  scans: number;
  lookups: number;
  updates: number;
  duplicate: boolean;
  duplicateOf?: string;
  unused: boolean;
  dropCandidate: boolean;
  keyColumns?: string[];
  includedColumns?: string[];
}

export interface StatHealth {
  name: string;
  lastUpdated?: string;
  rows: number;
  rowsSampled: number;
  modifications: number;
  pctSampled: number;
  pctChanged: number;
  stale: boolean;
}

export interface Fix {
  kind: string;
  schema: string;
  table: string;
  target: string;
  detail: string;
  sample?: string;
  sql: string;
  rollback?: string;
}

export interface RollbackEntry {
  id: number;
  createdAt: string;
  database: string;
  schema: string;
  table: string;
  objectName: string;
  action: string;
  reason: string;
  appliedSql: string;
  rollbackSql: string;
  restoredAt?: string;
}

export interface RollbacksResponse {
  database: string;
  rollbacks?: RollbackEntry[];
}

export interface FixJob {
  id: string;
  status: "running" | "done" | "failed" | "stopped";
  database: string;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  fixes?: Fix[];
  results?: FixResult[];
  summary: {
    total: number;
    applied: number;
    failed: number;
    rebuilds: number;
    reorganizes: number;
    updateStats: number;
    dropIndexes: number;
  };
}

export interface FixResult {
  fix: Fix;
  applied: boolean;
  error?: string;
  messages?: string[];
}
