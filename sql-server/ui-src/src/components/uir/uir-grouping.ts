import type { UIR, UIRField, UIRMethod, UIRRecord } from "../../types/uir";

export interface SchemaGroup {
  schema: string;
  tables: UIRRecord[];
  views: UIRRecord[];
  otherRecords: UIRRecord[];
  procedures: UIRMethod[];
  functions: UIRMethod[];
}

export function groupBySchema(uir: UIR | undefined | null): SchemaGroup[] {
  const groups = new Map<string, SchemaGroup>();
  const get = (schema: string): SchemaGroup => {
    let g = groups.get(schema);
    if (!g) {
      g = { schema, tables: [], views: [], otherRecords: [], procedures: [], functions: [] };
      groups.set(schema, g);
    }
    return g;
  };

  for (const rec of uir?.records ?? []) {
    const schema = rec.package ?? "";
    const g = get(schema);
    switch (rec.recordType) {
      case "sql:table":
        g.tables.push(rec);
        break;
      case "sql:view":
        g.views.push(rec);
        break;
      default:
        g.otherRecords.push(rec);
    }
  }

  for (const fn of uir?.functions ?? []) {
    const schema = fn.package ?? "";
    const g = get(schema);
    const kind = (fn.properties?.kind as string | undefined) ?? "function";
    if (kind === "procedure") g.procedures.push(fn);
    else g.functions.push(fn);
  }

  return Array.from(groups.values()).sort((a, b) => a.schema.localeCompare(b.schema));
}

export function identifierPath(parts: {
  package?: string;
  type?: string;
  method?: string;
  field?: string;
  extra?: string;
}): string {
  return [parts.package ?? "", parts.type ?? "", parts.method ?? "", parts.field ?? "", parts.extra ?? ""].join("::");
}

export type TreeItem =
  | { kind: "schema"; key: string; schema: string; group: SchemaGroup; children: TreeItem[] }
  | {
      kind: "section";
      key: string;
      schema: string;
      title: "Tables" | "Views" | "Procedures" | "Functions";
      count: number;
      children: TreeItem[];
    }
  | { kind: "record"; key: string; record: UIRRecord; schema: string; children: TreeItem[] }
  | { kind: "field"; key: string; field: UIRField; parent: UIRRecord; schema: string }
  | { kind: "method"; key: string; method: UIRMethod; schema: string };

export function groupsToTreeItems(groups: SchemaGroup[]): TreeItem[] {
  return groups.map((g) => {
    const sections: TreeItem[] = [];
    if (g.tables.length > 0) sections.push(section(g, "Tables", g.tables.map((r) => recordItem(g.schema, r))));
    if (g.views.length > 0) sections.push(section(g, "Views", g.views.map((r) => recordItem(g.schema, r))));
    if (g.procedures.length > 0) sections.push(section(g, "Procedures", g.procedures.map((m) => methodItem(g.schema, m))));
    if (g.functions.length > 0) sections.push(section(g, "Functions", g.functions.map((m) => methodItem(g.schema, m))));

    return {
      kind: "schema",
      key: identifierPath({ package: g.schema }),
      schema: g.schema,
      group: g,
      children: sections,
    } as TreeItem;
  });
}

function section(
  g: SchemaGroup,
  title: "Tables" | "Views" | "Procedures" | "Functions",
  children: TreeItem[],
): TreeItem {
  const extra = title === "Tables" ? "tables" : title === "Views" ? "views" : title === "Procedures" ? "procs" : "fns";
  return { kind: "section", key: identifierPath({ package: g.schema, extra }), schema: g.schema, title, count: children.length, children };
}

function recordItem(schema: string, record: UIRRecord): TreeItem {
  const extensions = record.extends ?? [];
  const triggers = extensions.filter((e) => e.recordType === "sql:trigger");
  const indexes = extensions.filter((e) => e.recordType === "sql:index");
  const fieldChildren: TreeItem[] = (record.fields ?? []).map((f) => ({
    kind: "field",
    key: identifierPath({ package: schema, type: record.type, field: f.field }),
    field: f,
    parent: record,
    schema,
  }));
  const triggerChildren: TreeItem[] = triggers.map((t) => ({
    kind: "record",
    key: identifierPath({ package: schema, type: record.type, extra: t.type }),
    record: t,
    schema,
    children: [],
  }));
  const indexChildren: TreeItem[] = indexes.map((idx) => ({
    kind: "record",
    key: identifierPath({ package: schema, type: record.type, extra: `idx:${idx.type}` }),
    record: idx,
    schema,
    children: [],
  }));
  return {
    kind: "record",
    key: identifierPath({ package: schema, type: record.type }),
    record,
    schema,
    children: [...fieldChildren, ...indexChildren, ...triggerChildren],
  };
}

function methodItem(schema: string, method: UIRMethod): TreeItem {
  return { kind: "method", key: identifierPath({ package: schema, method: method.method }), method, schema };
}

export function formatIdentifier(opts: {
  package?: string;
  type?: string;
  method?: string;
  parentType?: string;
}): string {
  const parts: string[] = [];
  if (opts.package) parts.push(opts.package);
  if (opts.parentType) parts.push(opts.parentType);
  if (opts.type) parts.push(opts.type);
  if (opts.method && !opts.type) parts.push(opts.method);
  return parts.join(".");
}
