import type { UseQueryResult } from "@tanstack/react-query";
import { Loader2, Search } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { DatabasePicker } from "../DatabasePicker";
import { Card, ErrorBox } from "../../pages/StatsTab";
import type { HealthView } from "./types";
import { inputCls, labelCls } from "./shared";

export function HealthScanPanel({
  database,
  setDatabase,
  configID,
  table,
  setTable,
  scanMode,
  setScanMode,
  limit,
  setLimit,
  query,
  detailedNeedsTable,
}: {
  database: string;
  setDatabase: (value: string) => void;
  configID: string;
  table: string;
  setTable: (value: string) => void;
  scanMode: string;
  setScanMode: (value: string) => void;
  limit: number;
  setLimit: (value: number) => void;
  query: UseQueryResult<HealthView, Error>;
  detailedNeedsTable: boolean;
}) {
  return (
    <Card title="Index health scan" icon={<Search size={14} />}>
      <div className="flex flex-wrap items-end gap-density-2">
        <label className={labelCls}>
          database
          <DatabasePicker
            configID={configID}
            value={database}
            onChange={setDatabase}
            emptyLabel="Current / bound database"
          />
        </label>
        <label className={labelCls}>
          table
          <input
            value={table}
            onChange={(e) => setTable(e.currentTarget.value)}
            placeholder="dbo.Table (optional)"
            className={inputCls + " w-[180px]"}
          />
        </label>
        <label className={labelCls}>
          scan
          <select
            value={scanMode}
            onChange={(e) => setScanMode(e.currentTarget.value)}
            className={inputCls}
          >
            <option value="LIMITED">LIMITED (fast)</option>
            <option value="SAMPLED">SAMPLED</option>
            <option value="DETAILED">DETAILED (table required)</option>
          </select>
        </label>
        <label className={labelCls}>
          top tables
          <input
            type="number"
            min={1}
            max={500}
            value={limit}
            onChange={(e) => setLimit(parseInt(e.currentTarget.value) || 20)}
            className={inputCls + " w-[80px]"}
          />
        </label>
        <Button
          size="sm"
          onClick={() => query.refetch()}
          disabled={query.isFetching || detailedNeedsTable}
        >
          {query.isFetching ? (
            <Loader2 size={12} className="spin" />
          ) : (
            <Search size={12} />
          )}{" "}
          Scan
        </Button>
      </div>
      {detailedNeedsTable && (
        <p className="mt-density-1 text-xs text-amber-600">
          DETAILED scan requires a table.
        </p>
      )}
      {query.error && <ErrorBox error={query.error as Error} />}
    </Card>
  );
}
