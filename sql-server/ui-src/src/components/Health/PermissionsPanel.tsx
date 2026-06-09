import type { UseQueryResult } from "@tanstack/react-query";
import { ShieldAlert } from "lucide-react";
import { Card, ErrorBox } from "../../pages/StatsTab";
import type { PermissionReport } from "./types";

export function PermissionsPanel({
  query,
}: {
  query: UseQueryResult<PermissionReport, Error>;
}) {
  if (query.error) return <ErrorBox error={query.error as Error} />;
  const report = query.data;
  if (!report) return null;
  const missing = report.categories.filter((c) => !c.granted);
  if (missing.length === 0 && !report.warnings?.length) {
    return null;
  }
  return (
    <Card
      title="Permission diagnostics"
      icon={<ShieldAlert size={14} />}
      className={
        missing.length
          ? "border-amber-300 bg-amber-50 dark:border-amber-700 dark:bg-amber-950/30"
          : ""
      }
    >
      {missing.length ? (
        <div className="grid gap-density-2 md:grid-cols-2">
          {missing.map((c) => (
            <div
              key={c.category}
              className="rounded-md border border-amber-300 bg-background/70 p-density-2 text-xs"
            >
              <div className="font-semibold text-amber-700 dark:text-amber-300">
                {c.label}
              </div>
              {c.missingPermissions?.length ? (
                <div className="mt-density-1 text-muted-foreground">
                  Missing: {c.missingPermissions.join(", ")}
                </div>
              ) : null}
              {c.note && (
                <div className="mt-density-1 italic text-muted-foreground">
                  {c.note}
                </div>
              )}
              {c.grantStatements?.length ? (
                <pre className="mt-density-1 overflow-auto rounded bg-muted/50 p-density-1 text-[11px]">
                  {c.grantStatements.join("\n")}
                </pre>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <p className="m-0 text-xs text-muted-foreground">
          All probed SQL Server capabilities are granted.
        </p>
      )}
      {report.warnings?.length ? (
        <ul className="mt-density-2 m-0 pl-4 text-xs text-amber-700 dark:text-amber-300">
          {report.warnings.map((w, i) => (
            <li key={i}>{w}</li>
          ))}
        </ul>
      ) : null}
    </Card>
  );
}
