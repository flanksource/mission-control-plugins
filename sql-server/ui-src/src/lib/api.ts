import { createEmbeddedPluginClient } from "@flanksource/mission-control-sdk";

// API client for the sql-server plugin's iframe.
//
// The iframe is served from `/api/plugins/sql-server/ui/` (the host's
// reverse proxy). Unary plugin operations are exposed by Mission Control at
// `/api/plugins/sql-server/invoke/<name>`.
//
// `config_id` is the catalog item the user is viewing — for the SQL Server
// plugin that's the Connection UUID (the host's resolveSQLConnection looks
// it up by ID). The invoke endpoint accepts it as a query param.

export const PLUGIN_NAME = "sql-server";

const pluginClient = createEmbeddedPluginClient({ name: PLUGIN_NAME });

// OpError carries the parsed error body alongside the message so the UI's
// ErrorDetails component (via normalizeErrorDiagnostics) can lift trace IDs,
// stack traces, and oops context out of structured error responses.
export class OpError extends Error {
  readonly status: number;
  readonly operation: string;
  readonly body: unknown;

  constructor(operation: string, status: number, message: string, body: unknown) {
    super(message);
    this.name = "OpError";
    this.operation = operation;
    this.status = status;
    this.body = body;
  }
}

export async function callOp<T = unknown>(
  op: string,
  _configID: string,
  params: Record<string, unknown> = {},
): Promise<T> {
  const res = await pluginClient.invoke(op, params);
  if (!res.ok) {
    const text = await res.text();
    let body: unknown = text;
    let message = text || res.statusText;
    try {
      const parsed = JSON.parse(text);
      body = parsed;
      if (parsed && typeof parsed === "object") {
        const record = parsed as Record<string, unknown>;
        const candidate = record.message ?? record.error ?? record.msg;
        if (typeof candidate === "string" && candidate) {
          message = candidate;
        }
      }
    } catch {
      // body is plain text — already captured above
    }
    throw new OpError(op, res.status, `${op} ${res.status}: ${message}`, body);
  }
  // The plugin SDK returns application/clicky+json — the payload is the
  // operation handler's JSON result. We parse it directly.
  return (await res.json()) as T;
}

export async function openTraceStream(
  traceID: string,
  onEvent: (e: unknown) => void,
  onDone?: () => void,
  since?: string,
  signal?: AbortSignal,
): Promise<void> {
  for await (const event of pluginClient.stream(
    "trace-stream",
    { id: traceID, since },
    { signal },
  )) {
    if (event.event === "done") {
      onDone?.();
      return;
    }
    if (event.event === "message") {
      let data: unknown;
      try {
        data = JSON.parse(event.data);
      } catch {
        continue;
      }
      onEvent(data);
    }
  }
}

export function configIDFromURL(): string {
  return new URLSearchParams(window.location.search).get("config_id") ?? "";
}
