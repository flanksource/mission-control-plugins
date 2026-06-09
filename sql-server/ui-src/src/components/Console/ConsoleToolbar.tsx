import { Loader2, Play, Wand2 } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { DatabasePicker } from "../DatabasePicker";
import type { HistoryEntry } from "../../lib/history";
import { HistoryButton } from "./HistoryButton";

interface ConsoleToolbarProps {
  configID: string;
  database: string;
  setDatabase: (database: string) => void;
  onRun: () => void;
  onExplain: () => void;
  running: boolean;
  explaining: boolean;
  history: HistoryEntry[];
  showHistory: boolean;
  onToggleHistory: () => void;
  onCloseHistory: () => void;
  onSelectHistory: (query: string) => void;
  onClearHistory: () => void;
}

export function ConsoleToolbar({
  configID,
  database,
  setDatabase,
  onRun,
  onExplain,
  running,
  explaining,
  history,
  showHistory,
  onToggleHistory,
  onCloseHistory,
  onSelectHistory,
  onClearHistory,
}: ConsoleToolbarProps) {
  return (
    <header className="flex flex-wrap items-center gap-density-2">
      <label className="flex items-center gap-density-1 text-xs text-muted-foreground">
        Database
        <DatabasePicker
          configID={configID}
          value={database}
          onChange={setDatabase}
          emptyLabel="Default database"
        />
      </label>
      <Button size="sm" onClick={onRun} disabled={running}>
        {running ? <Loader2 size={12} className="spin" /> : <Play size={12} />} Run
      </Button>
      <Button variant="outline" size="sm" onClick={onExplain} disabled={explaining}>
        {explaining ? <Loader2 size={12} className="spin" /> : <Wand2 size={12} />} Explain
      </Button>
      <HistoryButton
        history={history}
        open={showHistory}
        onToggle={onToggleHistory}
        onClose={onCloseHistory}
        onSelect={onSelectHistory}
        onClear={onClearHistory}
      />
      <span className="ml-auto text-xs text-muted-foreground">
        ⌘/Ctrl+Enter runs · ⌘/Ctrl+Shift+Enter explains
      </span>
    </header>
  );
}
