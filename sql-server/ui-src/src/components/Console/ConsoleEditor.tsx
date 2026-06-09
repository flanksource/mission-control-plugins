import { useEffect, useState } from "preact/hooks";
import Editor, { type OnMount } from "@monaco-editor/react";

interface ConsoleEditorProps {
  value: string;
  onChange: (value: string) => void;
  onMount: OnMount;
}

export function ConsoleEditor({ value, onChange, onMount }: ConsoleEditorProps) {
  const theme = useMonacoTheme();

  return (
    <div className="min-h-[160px] flex-[0_0_30%] overflow-hidden rounded-md border border-border bg-card">
      <Editor
        defaultLanguage="sql"
        theme={theme}
        value={value}
        onChange={(v) => onChange(v ?? "")}
        onMount={onMount}
        options={{
          minimap: { enabled: false },
          fontSize: 13,
          scrollBeyondLastLine: false,
          automaticLayout: true,
        }}
      />
    </div>
  );
}

function useMonacoTheme() {
  const [isDark, setIsDark] = useState(isDarkMode);

  useEffect(() => {
    const update = () => setIsDark(isDarkMode());
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    const observer = new MutationObserver(update);

    observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "data-theme"] });
    if (document.body) {
      observer.observe(document.body, { attributes: true, attributeFilter: ["class", "data-theme"] });
    }
    media?.addEventListener?.("change", update);
    update();

    return () => {
      observer.disconnect();
      media?.removeEventListener?.("change", update);
    };
  }, []);

  return isDark ? "vs-dark" : "vs";
}

function isDarkMode() {
  const roots = [document.documentElement, document.body].filter(Boolean);
  if (roots.some((el) => el.classList.contains("dark") || el.dataset.theme === "dark")) return true;
  if (roots.some((el) => el.classList.contains("light") || el.dataset.theme === "light")) return false;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}
