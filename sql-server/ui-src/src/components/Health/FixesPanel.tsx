import { Loader2, Wrench } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { Card, ErrorBox } from "../../pages/StatsTab";
import type { Fix } from "./types";
import {
  defaultSelectedFixIndexes,
  EmptyState,
  fixTone,
  monoTd,
  tdCls,
  thCls,
} from "./shared";

export function FixesPanel({
  fixes,
  selected,
  onToggle,
  onToggleAll,
  onApply,
  applying,
  applyError,
}: {
  fixes: Fix[];
  selected: Set<number>;
  onToggle: (i: number) => void;
  onToggleAll: () => void;
  onApply: () => void;
  applying: boolean;
  applyError: Error | null;
}) {
  return (
    <Card
      title={`Recommended fixes (${fixes.length})`}
      icon={<Wrench size={14} />}
    >
      {fixes.length === 0 ? (
        <EmptyState
          title="0 recommended fixes"
          body="This scan did not find fragmented indexes or stale statistics that need remediation."
        />
      ) : (
        <>
          <div className="mb-density-1 flex items-center gap-density-2 text-xs">
            <label className="inline-flex items-center gap-density-1">
              <input
                type="checkbox"
                checked={
                  selected.size > 0 &&
                  selected.size === defaultSelectedFixIndexes(fixes).length
                }
                onChange={onToggleAll}
              />{" "}
              select rebuild/reorg/stats
            </label>
            {fixes.some((f) => f.kind.includes("DROP")) && (
              <span className="text-muted-foreground">
                DROP INDEX recommendations are diagnostic-only until rollback
                restore is migrated.
              </span>
            )}
            <Button
              size="sm"
              disabled={applying || selected.size === 0}
              onClick={onApply}
            >
              {applying ? (
                <Loader2 size={12} className="spin" />
              ) : (
                <Wrench size={12} />
              )}{" "}
              Apply selected ({selected.size})
            </Button>
          </div>
          <div className="max-h-[360px] overflow-auto rounded-md border border-border">
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="bg-muted/30">
                  <th className={thCls}></th>
                  {"Kind Table Target Detail SQL".split(" ").map((h) => (
                    <th key={h} className={thCls}>
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {fixes.map((f, i) => {
                  const isDrop = f.kind.includes("DROP");
                  return (
                    <tr
                      key={`${f.kind}-${f.schema}-${f.table}-${f.target}-${i}`}
                      className="border-t border-border"
                    >
                      <td className={tdCls}>
                        <input
                          type="checkbox"
                          disabled={isDrop}
                          checked={selected.has(i)}
                          onChange={() => onToggle(i)}
                        />
                      </td>
                      <td className={tdCls + fixTone(f.kind)}>{f.kind}</td>
                      <td className={monoTd}>
                        {f.schema}.{f.table}
                      </td>
                      <td className={monoTd}>{f.target}</td>
                      <td className={tdCls}>{f.detail}</td>
                      <td className={monoTd}>
                        <code>{f.sql}</code>
                        {f.rollback && (
                          <div className="mt-1 text-muted-foreground">
                            rollback: {f.rollback}
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
      {applyError && <ErrorBox error={applyError} />}
    </Card>
  );
}
