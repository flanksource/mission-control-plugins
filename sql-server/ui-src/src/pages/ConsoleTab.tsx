import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { type OnMount } from "@monaco-editor/react";

// Avoid pulling monaco-editor types directly — it's a peer dep that
// the plugin's UI doesn't list. Derive the editor handle type from
// OnMount's first parameter instead.
type MonacoEditor = Parameters<OnMount>[0];
type Monaco = Parameters<OnMount>[1];
type Disposable = { dispose: () => void };

import { useMutation, useQuery } from "@tanstack/react-query";
import { callOp, configIDFromURL } from "../lib/api";
import {
  clearHistory,
  loadHistory,
  saveToHistory,
  type HistoryEntry,
} from "../lib/history";
import { registerSqlCompletion, type SchemaInfo } from "../lib/sql-completion";
import { ErrorBox, Card } from "./StatsTab";
import { ConsoleEditor } from "../components/Console/ConsoleEditor";
import { ConsoleResults, type QueryResult } from "../components/Console/ConsoleResults";
import { ConsoleToolbar } from "../components/Console/ConsoleToolbar";

interface ExplainResult {
  plan: string;
  format: string;
}

const DEFAULT_QUERY = "SELECT @@VERSION;\n";

// readDeepLink returns the seed statement and run flag from `?q=&run=1`,
// then strips both from the URL so a refresh does not re-execute.
function readDeepLink(): { seed: string | null; autoRun: boolean } {
  const params = new URLSearchParams(window.location.search);
  const seed = params.get("q");
  const autoRun = params.get("run") === "1";
  if (seed != null || params.has("run")) {
    params.delete("q");
    params.delete("run");
    const next = params.toString();
    const search = next ? `?${next}` : "";
    window.history.replaceState({}, "", window.location.pathname + search + window.location.hash);
  }
  return { seed, autoRun };
}

export function ConsoleTab() {
  const configID = configIDFromURL();
  const deepLink = useRef(readDeepLink()).current;
  const [statement, setStatement] = useState(deepLink.seed ?? DEFAULT_QUERY);
  const [database, setDatabase] = useState("");
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory);
  const [showHistory, setShowHistory] = useState(false);
  const editorRef = useRef<MonacoEditor | null>(null);
  const monacoRef = useRef<Monaco | null>(null);
  const completionRef = useRef<Disposable | null>(null);
  const pendingAutoRunRef = useRef(deepLink.autoRun && !!deepLink.seed);

  // Schema fetch is best-effort — autocomplete degrades gracefully to
  // keywords + functions when it fails. Re-fetches when the database the
  // user types changes (debounced via react-query's queryKey).
  const schemaQuery = useQuery({
    queryKey: ["schema", configID, database],
    queryFn: () => callOp<SchemaInfo>("schema", configID, { database }),
    staleTime: 5 * 60_000,
    retry: false,
  });

  const queryMut = useMutation({
    mutationFn: (toRun: string) =>
      callOp<QueryResult>("query", configID, { statement: toRun, database }),
  });
  const explainMut = useMutation({
    mutationFn: (toRun: string) =>
      callOp<ExplainResult>("explain", configID, {
        statement: toRun,
        database,
        format: "xml",
      }),
  });

  // executeQuery runs the editor selection if non-empty (so the user can
  // highlight one statement out of many), else the full buffer.
  const executeQuery = useCallback(
    (mode: "run" | "explain") => {
      const ed = editorRef.current;
      let toRun = statement;
      if (ed) {
        const sel = ed.getSelection();
        const model = ed.getModel();
        if (sel && model && !sel.isEmpty()) {
          toRun = model.getValueInRange(sel);
        } else {
          toRun = ed.getValue();
        }
      }
      const trimmed = toRun.trim();
      if (!trimmed) return;
      setHistory(saveToHistory(trimmed));
      if (mode === "run") queryMut.mutate(trimmed);
      else explainMut.mutate(trimmed);
    },
    [statement, queryMut, explainMut],
  );

  const handleEditorMount: OnMount = useCallback(
    (editor, monaco) => {
      editorRef.current = editor;
      monacoRef.current = monaco;
      editor.addAction({
        id: "sql-server-execute",
        label: "Execute SQL",
        keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter],
        run: () => executeQuery("run"),
      });
      editor.addAction({
        id: "sql-server-explain",
        label: "Explain SQL",
        keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.Enter],
        run: () => executeQuery("explain"),
      });
      if (schemaQuery.data) {
        completionRef.current = registerSqlCompletion(monaco, schemaQuery.data);
      }
      if (pendingAutoRunRef.current) {
        pendingAutoRunRef.current = false;
        // queueMicrotask so the editor's value is committed first.
        queueMicrotask(() => executeQuery("run"));
      }
    },
    [executeQuery, schemaQuery.data],
  );

  // Re-register the completion provider whenever the schema changes.
  // The mount-time branch above handles the first fetch when Monaco is
  // already up; this effect handles late arrivals + schema refresh.
  useEffect(() => {
    const monaco = monacoRef.current;
    const schema = schemaQuery.data;
    if (!monaco || !schema) return;
    completionRef.current?.dispose();
    completionRef.current = registerSqlCompletion(monaco, schema);
    return () => {
      completionRef.current?.dispose();
      completionRef.current = null;
    };
  }, [schemaQuery.data]);

  return (
    <section className="flex h-[calc(100vh-80px)] flex-col gap-density-2">
      <ConsoleToolbar
        configID={configID}
        database={database}
        setDatabase={setDatabase}
        onRun={() => executeQuery("run")}
        onExplain={() => executeQuery("explain")}
        running={queryMut.isPending}
        explaining={explainMut.isPending}
        history={history}
        showHistory={showHistory}
        onToggleHistory={() => setShowHistory((v) => !v)}
        onCloseHistory={() => setShowHistory(false)}
        onSelectHistory={(q) => {
          setStatement(q);
          editorRef.current?.setValue(q);
          setShowHistory(false);
        }}
        onClearHistory={() => {
          clearHistory();
          setHistory([]);
        }}
      />

      <ConsoleEditor value={statement} onChange={setStatement} onMount={handleEditorMount} />

      <div className="flex-1 overflow-auto">
        {queryMut.error && <ErrorBox error={queryMut.error as Error} />}
        {queryMut.data && <ConsoleResults result={queryMut.data} />}
        {explainMut.data && (
          <Card title="Execution plan (XML)" className="mt-density-2">
            <pre className="m-0 max-h-[360px] overflow-auto whitespace-pre-wrap font-mono text-[11px]">
              {explainMut.data.plan}
            </pre>
          </Card>
        )}
        {explainMut.error && <ErrorBox error={explainMut.error as Error} />}
      </div>
    </section>
  );
}
