import React, { useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { FileTree, useFileTree } from "@pierre/trees/react";
import { invoke as sdkInvoke, ready } from "@flanksource/plugin-ui-sdk";
import { logBanner } from "./version";
import "./styles.css";

logBanner();

type PrefixMetadata = {
  name: string;
  prefix: string;
};

type ObjectMetadata = {
  name: string;
  key: string;
  size: number;
  createdAt?: string;
  lastModified?: string;
  etag?: string;
  storageClass?: string;
};

type BucketListing = {
  bucket: string;
  prefix?: string;
  region?: string;
  endpoint?: string;
  usePathStyle?: boolean;
  createdAt?: string;
  objectCount: number;
  totalSize: number;
  isTruncated?: boolean;
  nextContinuationToken?: string;
  prefixes?: PrefixMetadata[];
  objects: ObjectMetadata[];
};

type ObjectContent = ObjectMetadata & {
  contentType?: string;
  content?: string;
  encoding?: "utf-8" | "base64" | string;
  bytesRead: number;
  truncated?: boolean;
  acceptRanges?: string;
  contentRange?: string;
};

type TreeEntry =
  | { type: "prefix"; path: string; prefix: PrefixMetadata }
  | { type: "object"; path: string; object: ObjectMetadata };

async function invoke<T>(operation: string, body: unknown) {
  const res = await sdkInvoke(operation, body);
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

function listBucket(prefix: string, continuationToken = "") {
  return invoke<BucketListing>("list-bucket", { prefix, continuationToken, maxKeys: 1000, delimiter: "/" });
}

function getObject(key: string) {
  return invoke<ObjectContent>("get-object", { key, maxBytes: 1024 * 1024 });
}

function formatBytes(size: number) {
  if (!Number.isFinite(size)) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatDate(value?: string) {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

function TreePane({
  bucket,
  loadingPrefix,
  paths,
  onSelect,
}: {
  bucket: string;
  loadingPrefix: string;
  paths: string[];
  onSelect: (path: string) => void;
}) {
  const knownPathsRef = useRef<Set<string>>(new Set());
  const loadingPrefixRef = useRef(loadingPrefix);
  const { model } = useFileTree({
    flattenEmptyDirectories: false,
    initialExpansion: "closed",
    onSelectionChange: (selected) => onSelect(selected[0] ?? ""),
    paths: [],
    renderRowDecoration: ({ row }) => {
      if (row.kind === "directory" && row.path === loadingPrefixRef.current) {
        return { text: "Loading…", title: "Loading folder" };
      }
      return null;
    },
    search: true,
  });

  useEffect(() => {
    const additions = paths.filter((path) => !knownPathsRef.current.has(path) && !model.getItem(path));
    if (additions.length > 0) {
      model.batch(additions.map((path) => ({ type: "add" as const, path })));
    }
    for (const path of paths) {
      knownPathsRef.current.add(path);
    }
  }, [model, paths]);

  useEffect(() => {
    loadingPrefixRef.current = loadingPrefix;
    model.setComposition(model.getComposition());
  }, [loadingPrefix, model]);

  return (
    <>
      <div className="tree-title">{bucket || "S3 bucket"}</div>
      <FileTree model={model} className="tree" />
    </>
  );
}

function App() {
  const [bucket, setBucket] = useState("");
  const [entries, setEntries] = useState<Record<string, TreeEntry>>({});
  const [loadedPrefixes, setLoadedPrefixes] = useState<Set<string>>(() => new Set());
  const [selectedPath, setSelectedPath] = useState("");
  const [selectedObject, setSelectedObject] = useState<ObjectContent | null>(null);
  const [loadingPrefix, setLoadingPrefix] = useState("");
  const [loadingObject, setLoadingObject] = useState(false);
  const [error, setError] = useState("");

  const paths = useMemo(() => Object.keys(entries).sort((a, b) => a.localeCompare(b)), [entries]);
  const selectedEntry = selectedPath ? entries[selectedPath] : undefined;

  async function loadPrefix(prefix: string) {
    if (loadedPrefixes.has(prefix)) return;

    setLoadingPrefix(prefix || "/");
    setError("");
    try {
      let token = "";
      const nextEntries: Record<string, TreeEntry> = {};
      let listing: BucketListing | null = null;
      do {
        listing = await listBucket(prefix, token);
        setBucket(listing.bucket);
        for (const obj of listing.objects ?? []) {
          if (obj.key !== prefix) {
            nextEntries[obj.key] = { type: "object", path: obj.key, object: obj };
          }
        }
        for (const dir of listing.prefixes ?? []) {
          nextEntries[dir.prefix] = { type: "prefix", path: dir.prefix, prefix: dir };
        }
        token = listing.nextContinuationToken ?? "";
      } while (listing?.isTruncated && token);

      setEntries((prev) => ({ ...prev, ...nextEntries }));
      setLoadedPrefixes((prev) => new Set(prev).add(prefix));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoadingPrefix("");
      ready();
    }
  }

  useEffect(() => {
    void loadPrefix("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    setSelectedObject(null);
    if (!selectedEntry) return;
    if (selectedEntry.type === "prefix") {
      void loadPrefix(selectedEntry.prefix.prefix);
      return;
    }

    let cancelled = false;
    setLoadingObject(true);
    setError("");
    getObject(selectedEntry.object.key)
      .then((obj) => {
        if (!cancelled) setSelectedObject(obj);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoadingObject(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedPath]);

  return (
    <main className="app">
      {error && <div className="error">{error}</div>}
      <section className="split">
        <aside className="left-pane">
          <TreePane bucket={bucket} loadingPrefix={loadingPrefix} paths={paths} onSelect={setSelectedPath} />
          {loadingPrefix && <div className="status">Loading {loadingPrefix}…</div>}
        </aside>
        <section className="right-pane">
          {!selectedEntry && <div className="empty-state">Select a file or folder.</div>}

          {selectedEntry?.type === "prefix" && (
            <div className="details">
              <h2>{selectedEntry.prefix.name || selectedEntry.prefix.prefix}</h2>
              <dl>
                <dt>Prefix</dt><dd>{selectedEntry.prefix.prefix}</dd>
                <dt>Status</dt><dd>{loadedPrefixes.has(selectedEntry.prefix.prefix) ? "Loaded" : "Loading…"}</dd>
              </dl>
            </div>
          )}

          {selectedEntry?.type === "object" && (
            <div className="details">
              <h2>{selectedEntry.object.name}</h2>
              <dl>
                <dt>Key</dt><dd>{selectedEntry.object.key}</dd>
                <dt>Size</dt><dd>{formatBytes(selectedEntry.object.size)}</dd>
                <dt>Modified</dt><dd>{formatDate(selectedEntry.object.lastModified ?? selectedEntry.object.createdAt)}</dd>
                <dt>Storage</dt><dd>{selectedEntry.object.storageClass || "—"}</dd>
                <dt>ETag</dt><dd>{selectedEntry.object.etag || "—"}</dd>
                {selectedObject?.contentType && <><dt>Content type</dt><dd>{selectedObject.contentType}</dd></>}
              </dl>

              <h3>Preview</h3>
              {loadingObject && <div className="status">Loading object…</div>}
              {!loadingObject && selectedObject?.encoding === "base64" && <div className="empty-state">Binary preview unavailable ({formatBytes(selectedObject.bytesRead)} read).</div>}
              {!loadingObject && selectedObject?.encoding !== "base64" && (
                <pre className="preview">{selectedObject?.content || ""}</pre>
              )}
              {selectedObject?.truncated && <div className="status">Preview truncated at {formatBytes(selectedObject.bytesRead)}.</div>}
            </div>
          )}
        </section>
      </section>
    </main>
  );
}

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
createRoot(root).render(<App />);
