export type GadgetSpec = {
  id: string;
  name: string;
  image: string;
  description: string;
  kind: string;
  widget: GadgetWidget;
  category: string;
  icon: string;
  docsUrl: string;
  streaming: boolean;
  options?: GadgetOption[];
  eventSchema?: EventSchema;
};

export type GadgetWidget = "trace" | "top" | "snapshot" | "profile" | "report" | "table";

export type EventSchema = {
  sourceStruct?: string;
  columns?: EventColumnSpec[];
};

export type EventColumnSpec = {
  key: string;
  label: string;
  path: string;
  kind?: string;
  description?: string;
  hidden?: boolean;
};

export type GadgetOption = {
  name: string;
  type: string;
  description?: string;
  default?: unknown;
};

export type Status = {
  namespace: string;
  installed: boolean;
  ready: boolean;
  version?: string;
  desired?: number;
  readyPods?: number;
  problems?: string[];
  manifest?: string;
};

export type Session = {
  id: string;
  gadgetId: string;
  gadgetName: string;
  gadgetKind: string;
  gadgetWidget?: GadgetWidget;
  gadgetImage: string;
  gadgetTag: string;
  docsUrl?: string;
  state: string;
  target: {
    namespace: string;
    kind?: string;
    name?: string;
    pod?: string;
    container?: string;
    node?: string;
  };
  startedAt: string;
  stoppedAt?: string;
  error?: string;
  eventCount: number;
  diagnostics: {
    runtime: string;
    connection: string;
    gadgetWidget?: GadgetWidget;
    gadgetImage: string;
    gadgetTag: string;
    gadgetDocsUrl?: string;
    durationSec: number;
    maxEvents: number;
    maxSessions: number;
    resolvedPods?: Array<{ namespace: string; name: string; node?: string; containers: string[] }>;
    runtimeParams?: Record<string, string>;
    userOptions?: Record<string, unknown>;
    startedByEmail?: string;
  };
};

export type TraceEvent = {
  sessionId: string;
  sequence: number;
  time: string;
  node?: string;
  raw?: string;
  error?: string;
  data?: Record<string, unknown>;
};

export type EventTableRow = TraceEvent & {
  timeLabel: string;
  summary: string;
  __rowKey?: string;
  __sampleCount?: number;
};
