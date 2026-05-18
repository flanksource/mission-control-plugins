// API client for the postgres plugin's iframe.
//
// The iframe is served from `/api/plugins/postgres/ui/` (the host's
// reverse proxy). Unary plugin operations are exposed by Mission Control at
// `/api/plugins/postgres/invoke/<name>`.
//
// `config_id` is the catalog item the user is viewing. The invoke endpoint
// accepts it as a query param.

export const PLUGIN_NAME = "postgres";

function pluginBasePath(): string {
  const match = window.location.pathname.match(/^(.*\/api\/plugins\/[^/]+)\/ui(?:\/.*)?$/);
  if (match) return match[1];
  return `/api/plugins/${PLUGIN_NAME}`;
}

function operationURL(op: string, configID: string): string {
  const url = new URL(
    `${pluginBasePath()}/invoke/${encodeURIComponent(op)}`,
    window.location.origin,
  );
  if (configID) url.searchParams.set("config_id", configID);
  return url.toString();
}

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
  configID: string,
  params: Record<string, unknown> = {},
): Promise<T> {
  const res = await fetch(operationURL(op, configID), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify(params),
  });
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

export function configIDFromURL(): string {
  return new URLSearchParams(window.location.search).get("config_id") ?? "";
}
