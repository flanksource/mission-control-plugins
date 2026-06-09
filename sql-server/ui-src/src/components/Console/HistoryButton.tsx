import { History, Trash2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import type { HistoryEntry } from "../../lib/history";

interface HistoryButtonProps {
  history: HistoryEntry[];
  open: boolean;
  onToggle: () => void;
  onClose: () => void;
  onSelect: (query: string) => void;
  onClear: () => void;
}

export function HistoryButton({ history, open, onToggle, onClose, onSelect, onClear }: HistoryButtonProps) {
  return (
    <div className="relative">
      <Button variant="outline" size="sm" onClick={onToggle}>
        <History size={12} /> History
      </Button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={onClose} />
          <div className="absolute left-0 top-full z-50 mt-1 max-h-80 w-[28rem] overflow-auto rounded-md border border-border bg-popover shadow-md">
            {history.length === 0 ? (
              <div className="p-density-2 text-xs text-muted-foreground">No query history</div>
            ) : (
              <>
                <div className="flex items-center justify-between border-b border-border bg-muted/40 px-density-2 py-density-1">
                  <span className="text-[11px] uppercase tracking-wide text-muted-foreground">
                    {history.length} previous queries
                  </span>
                  <button
                    type="button"
                    onClick={onClear}
                    className="inline-flex items-center gap-density-1 text-[11px] text-muted-foreground hover:text-destructive"
                  >
                    <Trash2 size={11} /> Clear
                  </button>
                </div>
                {history.map((entry) => (
                  <button
                    key={entry.timestamp}
                    type="button"
                    className="block w-full truncate border-b border-border px-density-2 py-density-1 text-left font-mono text-xs last:border-0 hover:bg-accent"
                    onClick={() => onSelect(entry.query)}
                    title={entry.query}
                  >
                    {entry.query}
                  </button>
                ))}
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
}
