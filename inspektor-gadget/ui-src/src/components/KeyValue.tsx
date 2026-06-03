import { cn } from "@flanksource/clicky-ui";

export function KeyValue({ label, value, mono = false }: { label: string; value?: string; mono?: boolean }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 text-xs">
      <span className="text-[11px] font-semibold uppercase text-muted-foreground">{label}</span>
      <code className={cn("truncate", !mono && "font-sans")}>{value || ""}</code>
    </div>
  );
}
