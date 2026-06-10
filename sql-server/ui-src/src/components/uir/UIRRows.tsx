import type { ReactNode } from "preact/compat";
import { Eye, Key, Link2, ListTree, Table as TableIcon, Zap } from "lucide-react";
import type { UIRField, UIRRecord } from "../../types/uir";

export function RecordBadges({ record }: { record: UIRRecord }) {
  const badges: ReactNode[] = [];
  if (record.recordType === "sql:trigger") {
    const event = record.properties?.event as string | undefined;
    const timing = record.properties?.timing as string | undefined;
    if (timing || event) {
      badges.push(
        <span key="trg" className="rounded bg-amber-500/10 px-1 text-amber-600">
          {[timing, event].filter(Boolean).join(" ")}
        </span>,
      );
    }
  }
  if (record.recordType === "sql:view") {
    badges.push(
      <span key="view" className="rounded bg-slate-500/10 px-1 text-slate-600">
        view
      </span>,
    );
  }
  const refCount = record.references?.length ?? 0;
  if (refCount > 0 && record.recordType === "sql:table") {
    badges.push(
      <span key="fk" className="text-muted-foreground">
        {refCount} FK
      </span>,
    );
  }
  return <span className="ml-1 flex items-center gap-1">{badges}</span>;
}

export function FieldBadges({ field }: { field: UIRField }) {
  const badges: ReactNode[] = [];
  if (field.properties?.primaryKey) {
    badges.push(<Key key="pk" size={12} className="text-amber-500" aria-label="primary key" />);
  }
  if (field.properties?.foreignKey) {
    badges.push(<Link2 key="fk" size={12} className="text-blue-500" aria-label="foreign key" />);
  }
  if (field.readOnly) {
    badges.push(
      <span key="ro" className="text-[9px] uppercase text-muted-foreground">
        ro
      </span>,
    );
  }
  return <span className="flex items-center gap-1">{badges}</span>;
}

export function iconForRecord(rec: UIRRecord): ReactNode {
  switch (rec.recordType) {
    case "sql:table":
      return <TableIcon size={12} className="text-emerald-500" />;
    case "sql:view":
      return <Eye size={12} className="text-slate-500" />;
    case "sql:trigger":
      return <Zap size={12} className="text-amber-500" />;
    default:
      return <ListTree size={12} className="text-muted-foreground" />;
  }
}

export function iconForField(field: UIRField): ReactNode {
  if (field.fieldType === "array") return <ListTree size={12} className="text-sky-500" />;
  return null;
}
