import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import type { MouseEvent, ReactNode } from "preact/compat";
import { Tree } from "@flanksource/clicky-ui";
import { Cog, Database, Eye, FunctionSquare, Table as TableIcon } from "lucide-react";
import type { UIR, UIRNode } from "../../types/uir";
import { groupBySchema, groupsToTreeItems, type TreeItem } from "./uir-grouping";
import { FieldBadges, RecordBadges, iconForField, iconForRecord } from "./UIRRows";

export interface UIRAction {
  label: string;
  icon?: ReactNode;
  visible?: (node: UIRNode) => boolean;
  onClick: (node: UIRNode) => void;
}

export interface UIRExplorerProps {
  uir: UIR | undefined | null;
  actions?: UIRAction[];
  onSelect?: (node: UIRNode) => void;
  className?: string;
  empty?: ReactNode;
}

interface MenuState {
  x: number;
  y: number;
  node: UIRNode;
}

function treeItemToUIRNode(item: TreeItem): UIRNode | null {
  switch (item.kind) {
    case "schema":
      return { kind: "schema", schema: item.schema };
    case "record":
      return { kind: "record", record: item.record, schema: item.schema };
    case "field":
      return { kind: "field", field: item.field, parent: item.parent, schema: item.schema };
    case "method":
      return { kind: "method", method: item.method, schema: item.schema };
    default:
      return null;
  }
}

export function UIRExplorer({ uir, actions, onSelect, className = "", empty }: UIRExplorerProps) {
  const roots = useMemo(() => groupsToTreeItems(groupBySchema(uir)), [uir]);
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [selected, setSelected] = useState<TreeItem | null>(null);

  const handleContext = (e: MouseEvent<HTMLElement>, item: TreeItem) => {
    if (!actions || actions.length === 0) return;
    const node = treeItemToUIRNode(item);
    if (!node) return;
    if (!actions.some((a) => !a.visible || a.visible(node))) return;
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, node });
  };

  const handleSelect = (item: TreeItem) => {
    setSelected(item);
    if (item.kind === "section") return;
    const node = treeItemToUIRNode(item);
    if (node) onSelect?.(node);
  };

  if (roots.length === 0) {
    return (
      <>
        {empty ?? (
          <div className="rounded-xl border border-dashed p-density-4 text-center text-sm text-muted-foreground">
            No schemas found
          </div>
        )}
      </>
    );
  }

  return (
    <div className={`flex min-h-0 flex-1 flex-col font-mono text-xs ${className}`}>
      <Tree<TreeItem>
        className="min-h-0 flex-1"
        roots={roots}
        getKey={(n) => n.key}
        getChildren={(n) => ("children" in n ? n.children : undefined)}
        selected={selected}
        onSelect={handleSelect}
        defaultOpen={(n) => n.kind === "schema" || n.kind === "section"}
        isSecondary={isSecondaryNode}
        getSearchText={treeItemSearchText}
        renderRow={({ node }) => (
          <span className="flex min-w-0 flex-1 items-center gap-1.5" onContextMenu={(e) => handleContext(e, node)}>
            {renderRowContent(node)}
          </span>
        )}
      />
      {menu && actions && <ContextMenu state={menu} actions={actions} onClose={() => setMenu(null)} />}
    </div>
  );
}

function isSecondaryNode(node: TreeItem): boolean {
  if (node.kind === "field") return true;
  if (node.kind === "record" && node.record.recordType === "sql:index") return true;
  return false;
}

function treeItemSearchText(node: TreeItem): string {
  switch (node.kind) {
    case "schema":
      return node.schema;
    case "section":
      return node.title;
    case "record":
      return node.record.type ?? "";
    case "field":
      return node.field.field ?? "";
    case "method":
      return node.method.method ?? "";
  }
}

function renderRowContent(node: TreeItem): ReactNode {
  switch (node.kind) {
    case "schema":
      return (
        <>
          <Database size={14} className="shrink-0 text-blue-500" />
          <span className="truncate font-semibold">{node.schema || "(default)"}</span>
          <span className="ml-1 text-muted-foreground">
            {node.group.tables.length + node.group.views.length} objects, {node.group.procedures.length + node.group.functions.length} routines
          </span>
        </>
      );
    case "section":
      return (
        <>
          <SectionIcon title={node.title} />
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground">{node.title}</span>
          <span className="text-muted-foreground/70">({node.count})</span>
        </>
      );
    case "record":
      return (
        <>
          {iconForRecord(node.record)}
          <span className="truncate font-medium">{node.record.type}</span>
          <RecordBadges record={node.record} />
        </>
      );
    case "field":
      return (
        <>
          {iconForField(node.field)}
          <span className="truncate">{node.field.field}</span>
          <span className="truncate text-muted-foreground">{node.field.typeRef?.name ?? node.field.fieldType}</span>
          <FieldBadges field={node.field} />
        </>
      );
    case "method":
      return (
        <>
          {(node.method.properties?.kind as string | undefined) === "procedure" ? (
            <Cog size={12} className="shrink-0 text-purple-500" />
          ) : (
            <FunctionSquare size={12} className="shrink-0 text-teal-500" />
          )}
          <span className="truncate font-medium">{node.method.method}</span>
          <span className="text-muted-foreground">({(node.method.params ?? []).length})</span>
        </>
      );
  }
}

function SectionIcon({ title }: { title: "Tables" | "Views" | "Procedures" | "Functions" }) {
  switch (title) {
    case "Tables":
      return <TableIcon size={12} className="shrink-0" />;
    case "Views":
      return <Eye size={12} className="shrink-0" />;
    case "Procedures":
      return <Cog size={12} className="shrink-0" />;
    case "Functions":
      return <FunctionSquare size={12} className="shrink-0" />;
  }
}

function ContextMenu({ state, actions, onClose }: { state: MenuState; actions: UIRAction[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const onDown = (e: globalThis.MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  const visible = actions.filter((a) => !a.visible || a.visible(state.node));
  if (visible.length === 0) return null;

  return (
    <div
      ref={ref}
      role="menu"
      className="fixed z-50 min-w-[12rem] rounded-md border border-border bg-popover py-1 text-popover-foreground shadow-md shadow-black/10"
      style={{ top: state.y, left: state.x }}
    >
      {visible.map((a, i) => (
        <button
          key={i}
          type="button"
          role="menuitem"
          className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-accent"
          onClick={() => {
            a.onClick(state.node);
            onClose();
          }}
        >
          {a.icon}
          {a.label}
        </button>
      ))}
    </div>
  );
}
