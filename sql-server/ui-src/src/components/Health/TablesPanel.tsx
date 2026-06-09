import { Card } from "../../pages/StatsTab";
import { formatBytes, formatNumber, formatPercent } from "../../lib/format";
import type { TableHealth } from "./types";
import { Mark, monoTd, tableKey, tdCls, thCls } from "./shared";

export function TablesPanel({
  tables,
  usageReliable,
  selected,
  onToggle,
  onToggleAll,
}: {
  tables: TableHealth[];
  usageReliable: boolean;
  selected: Set<string>;
  onToggle: (key: string) => void;
  onToggleAll: () => void;
}) {
  if (!tables.length) {
    return (
      <Card title="Tables">
        <em className="text-muted-foreground">no matching tables</em>
      </Card>
    );
  }
  return (
    <Card title={`Tables (${tables.length})`}>
      <div className="mb-density-1 flex items-center gap-density-2 text-xs">
        <label className="inline-flex items-center gap-density-1">
          <input
            type="checkbox"
            checked={selected.size === tables.length}
            onChange={onToggleAll}
          />{" "}
          select all for bulk
        </label>
        {!usageReliable && (
          <span className="text-amber-600">
            unused-index flags need verification: usage counters may have reset
          </span>
        )}
      </div>
      <div className="overflow-auto">
        <table className="w-full border-collapse text-xs">
          <thead>
            <tr className="bg-muted/30">
              <th className={thCls}></th>
              {"Table Rows Total Index Unused MaxFrag Frag Stats"
                .split(" ")
                .map((h) => (
                  <th key={h} className={thCls}>
                    {h}
                  </th>
                ))}
            </tr>
          </thead>
          <tbody>
            {tables.map((t) => {
              const key = tableKey(t);
              return (
                <tr key={key} className="border-t border-border">
                  <td className={tdCls}>
                    <input
                      type="checkbox"
                      checked={selected.has(key)}
                      onChange={() => onToggle(key)}
                    />
                  </td>
                  <td className={monoTd}>
                    <details>
                      <summary className="cursor-pointer">{key}</summary>
                      <IndexDetails table={t} />
                    </details>
                  </td>
                  <td className={monoTd}>{formatNumber(t.rows)}</td>
                  <td className={monoTd}>{formatBytes(t.totalBytes)}</td>
                  <td className={monoTd}>{formatBytes(t.indexBytes)}</td>
                  <td className={monoTd}>{formatBytes(t.unusedBytes)}</td>
                  <td className={monoTd}>
                    {formatPercent(t.maxFragmentation)}
                  </td>
                  <td className={tdCls}>
                    <Mark ok={t.fragHealthy} />
                  </td>
                  <td className={tdCls}>
                    <Mark ok={t.statsHealthy} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}

function IndexDetails({ table }: { table: TableHealth }) {
  return (
    <div className="mt-density-1 grid gap-density-1 text-[11px] text-muted-foreground">
      {(table.indexes ?? []).length > 0 && (
        <div>
          <strong>Indexes</strong>
          <ul className="m-0 mt-1 pl-4">
            {(table.indexes ?? []).map((ix) => (
              <li key={ix.name}>
                <span
                  className={ix.bad || ix.dropCandidate ? "text-amber-600" : ""}
                >
                  {ix.name}
                </span>{" "}
                · {formatPercent(ix.fragmentation)} · {formatBytes(ix.bytes)} ·
                reads {formatNumber(ix.seeks + ix.scans + ix.lookups)} · writes{" "}
                {formatNumber(ix.updates)}
                {ix.duplicate &&
                  ` · duplicate of ${ix.duplicateOf || "wider index"}`}
                {ix.unused && " · unused?"}
              </li>
            ))}
          </ul>
        </div>
      )}
      {(table.statistics ?? []).length > 0 && (
        <div>
          <strong>Statistics</strong>
          <ul className="m-0 mt-1 pl-4">
            {(table.statistics ?? []).map((s) => (
              <li key={s.name} className={s.stale ? "text-amber-600" : ""}>
                {s.name} · changed {formatPercent(s.pctChanged)} · sampled{" "}
                {formatPercent(s.pctSampled)} · updated{" "}
                {s.lastUpdated
                  ? new Date(s.lastUpdated).toLocaleDateString()
                  : "never"}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
