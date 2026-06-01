import type { ComponentChildren } from "preact";
import { useEffect, useState } from "preact/hooks";
import { RefreshCw } from "lucide-react";
import { Badge, Button } from "@flanksource/clicky-ui";
import type { ActiveTab } from "./types";
import { errorMessage } from "./utils";

export function TabButton({
  tab,
  current,
  onClick,
  children,
}: {
  tab: ActiveTab;
  current: ActiveTab;
  onClick: (tab: ActiveTab) => void;
  children: ComponentChildren;
}) {
  return (
    <Button size="sm" variant={current === tab ? "secondary" : "ghost"} onClick={() => onClick(tab)}>
      {children}
    </Button>
  );
}

export function InfoCard({ title, children }: { title: string; children: ComponentChildren }) {
  return (
    <div className="rounded-md border bg-muted/20 p-3">
      <h4 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{title}</h4>
      <dl className="grid grid-cols-[8rem_1fr] gap-y-1 text-xs">{children}</dl>
    </div>
  );
}

export function KV({ k, v }: { k: string; v?: string }) {
  return (
    <>
      <dt className="text-muted-foreground">{k}</dt>
      <dd className="truncate">{v || "unknown"}</dd>
    </>
  );
}

export function Field({ label, children }: { label: string; children: ComponentChildren }) {
  return (
    <label className="flex flex-col gap-1 text-xs">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export function Empty({ children }: { children: ComponentChildren }) {
  return <div className="flex h-full items-center justify-center p-6 text-center text-sm text-muted-foreground">{children}</div>;
}

export function ErrorText({ error }: { error: unknown }) {
  return <div className="rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-700">{errorMessage(error)}</div>;
}

export function GopsRequiredOverlay({ children = "gops is required for this view." }: { children?: ComponentChildren }) {
  return (
    <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/70 p-6 backdrop-blur-[2px]">
      <div className="rounded-md border bg-card px-4 py-3 text-center text-sm font-medium shadow-sm">
        {children}
      </div>
    </div>
  );
}

export function LoadingOverlay({ children = "Loading…" }: { children?: ComponentChildren }) {
  return (
    <div className="absolute inset-0 z-20 flex items-center justify-center bg-background/60 p-6 backdrop-blur-[2px]">
      <div className="flex items-center gap-3 rounded-md border bg-card px-4 py-3 text-sm font-medium shadow-sm">
        <span className="h-4 w-4 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-primary" />
        {children}
      </div>
    </div>
  );
}

export function RefetchIndicator({ children = "Refreshing…" }: { children?: ComponentChildren }) {
  return (
    <div className="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border bg-card/95 px-3 py-1.5 text-xs font-medium shadow-sm">
      <RefreshCw className="h-3.5 w-3.5 animate-spin text-primary" />
      {children}
    </div>
  );
}

export function useDelayedTruthy(value: boolean, delayMs = 600) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!value) {
      setVisible(false);
      return;
    }

    const timeout = window.setTimeout(() => setVisible(true), delayMs);
    return () => window.clearTimeout(timeout);
  }, [value, delayMs]);

  return visible;
}

export function RunBadge({ run }: { run: { state: string } }) {
  const tone = run.state === "completed" ? "success" : run.state === "failed" ? "danger" : run.state === "stopped" ? "warning" : "info";
  return <Badge tone={tone} variant="soft" size="sm">{run.state}</Badge>;
}
