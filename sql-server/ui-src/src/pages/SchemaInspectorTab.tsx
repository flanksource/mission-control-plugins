import { useMemo, useState } from "preact/hooks";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@flanksource/clicky-ui";
import { Copy, Database, RefreshCw } from "lucide-react";
import { callOp, configIDFromURL } from "../lib/api";
import { ErrorBox } from "./StatsTab";
import { DatabasePicker } from "../components/DatabasePicker";
import { UIRExplorer, type UIRAction } from "../components/uir/UIRExplorer";
import { formatIdentifier, groupBySchema } from "../components/uir/uir-grouping";
import type { UIR, UIRNode } from "../types/uir";

export function SchemaInspectorTab() {
  const configID = configIDFromURL();
  const [database, setDatabase] = useState("");
  const [refreshToken, setRefreshToken] = useState(0);

  const { data, isLoading, isFetching, error } = useQuery({
    queryKey: ["inspect", configID, database, refreshToken],
    queryFn: () =>
      callOp<UIR>("inspect", configID, {
        ...(database ? { database } : {}),
        ...(refreshToken > 0 ? { refresh: true } : {}),
      }),
    staleTime: 24 * 60 * 60_000,
  });

  const summary = useMemo(() => summarize(data), [data]);

  const actions: UIRAction[] = [
    {
      label: "Copy identifier",
      icon: <Copy size={14} />,
      onClick: (node) => {
        const text = identifierFor(node);
        if (text) void navigator.clipboard?.writeText(text);
      },
    },
  ];

  return (
    <section className="flex h-[calc(100vh-7rem)] min-h-[32rem] flex-col">
      <header className="mb-density-2 border-b border-border pb-density-3">
        <h3 className="m-0 inline-flex items-center gap-density-1">
          <Database size={16} /> Schema Inspector
        </h3>
        <p className="m-0 mt-1 text-xs text-muted-foreground">
          Browse schemas, tables, views, columns, indexes, triggers, functions, and procedures for this Mission Control config item.
        </p>
        <div className="mt-density-2 flex flex-wrap items-center gap-density-2">
          <span className="text-xs font-medium text-muted-foreground">Database</span>
          <DatabasePicker
            configID={configID}
            value={database}
            onChange={(next) => {
              setDatabase(next);
              setRefreshToken(0);
            }}
            emptyLabel="Default database"
            disabled={isFetching}
          />
          <Button variant="outline" size="sm" onClick={() => setRefreshToken((n) => n + 1)} disabled={isFetching}>
            <RefreshCw size={12} className={isFetching ? "spin" : ""} /> Refresh
          </Button>
        </div>
      </header>

      {summary && (
        <div className="mb-density-2 grid gap-density-2 text-xs [grid-template-columns:repeat(auto-fit,minmax(140px,1fr))]">
          <SummaryCard label="Schemas" value={summary.schemas} />
          <SummaryCard label="Tables" value={summary.tables} />
          <SummaryCard label="Views" value={summary.views} />
          <SummaryCard label="Routines" value={summary.routines} />
        </div>
      )}

      {isLoading && <p className="text-sm text-muted-foreground">Introspecting schema…</p>}
      {error && <ErrorBox error={error as Error} />}

      {data && !isLoading && (
        <div className="flex min-h-0 flex-1 rounded-lg border border-border bg-card p-density-2">
          <UIRExplorer uir={data} actions={actions} />
        </div>
      )}
    </section>
  );
}

function SummaryCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-border bg-card p-density-2">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}

function summarize(uir: UIR | undefined) {
  if (!uir) return null;
  const groups = groupBySchema(uir);
  return {
    schemas: groups.length,
    tables: groups.reduce((n, g) => n + g.tables.length, 0),
    views: groups.reduce((n, g) => n + g.views.length, 0),
    routines: groups.reduce((n, g) => n + g.procedures.length + g.functions.length, 0),
  };
}

function identifierFor(node: UIRNode): string {
  switch (node.kind) {
    case "schema":
      return node.schema;
    case "record":
      return formatIdentifier({ package: node.schema, type: node.record.type });
    case "method":
      return formatIdentifier({ package: node.schema, method: node.method.method });
    case "field":
      return [node.schema, node.parent.type, node.field.field].filter(Boolean).join(".");
  }
}
