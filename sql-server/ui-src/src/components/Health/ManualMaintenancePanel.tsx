import { Hammer, Loader2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { Card, ErrorBox } from "../../pages/StatsTab";

export function ManualMaintenancePanel({
  bulkRebuildIndexes,
  setBulkRebuildIndexes,
  bulkUpdateStats,
  setBulkUpdateStats,
  selectedTableRefs,
  bulk,
}: {
  bulkRebuildIndexes: boolean;
  setBulkRebuildIndexes: (value: boolean) => void;
  bulkUpdateStats: boolean;
  setBulkUpdateStats: (value: boolean) => void;
  selectedTableRefs: { schema: string; table: string }[];
  bulk: {
    isPending: boolean;
    error: Error | null;
    mutate: (tables: { schema: string; table: string }[]) => void;
  };
}) {
  return (
    <Card title="Manual maintenance (optional)" icon={<Hammer size={14} />}>
      <div className="grid gap-density-2">
        <p className="m-0 text-sm text-muted-foreground">
          Use this only when you want to force maintenance on selected tables,
          even if the scan has no recommended fixes. It queues table-wide{" "}
          <code>ALTER INDEX ALL ... REBUILD</code> and/or
          <code> UPDATE STATISTICS</code> jobs for the tables selected above.
        </p>
        <div className="flex flex-wrap items-center gap-density-2 text-xs">
          <label className="inline-flex items-center gap-density-1">
            <input
              type="checkbox"
              checked={bulkRebuildIndexes}
              onChange={(e) => setBulkRebuildIndexes(e.currentTarget.checked)}
            />
            rebuild indexes
          </label>
          <label className="inline-flex items-center gap-density-1">
            <input
              type="checkbox"
              checked={bulkUpdateStats}
              onChange={(e) => setBulkUpdateStats(e.currentTarget.checked)}
            />
            update stats
          </label>
          <Button
            size="sm"
            variant="outline"
            disabled={
              bulk.isPending ||
              selectedTableRefs.length === 0 ||
              (!bulkRebuildIndexes && !bulkUpdateStats)
            }
            onClick={() => bulk.mutate(selectedTableRefs)}
          >
            {bulk.isPending ? (
              <Loader2 size={12} className="spin" />
            ) : (
              <Hammer size={12} />
            )}{" "}
            Queue for {selectedTableRefs.length} table(s)
          </Button>
          <span className="text-muted-foreground">
            {selectedTableRefs.length === 0
              ? "Select one or more tables above to enable this."
              : "Ready to queue selected tables."}
          </span>
        </div>
      </div>
      {bulk.error && <ErrorBox error={bulk.error as Error} />}
    </Card>
  );
}
