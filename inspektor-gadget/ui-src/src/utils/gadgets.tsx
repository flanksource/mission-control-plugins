import type React from "react";
import {
  Activity,
  Cpu,
  FileSearch,
  Gauge,
  Globe,
  HardDrive,
  Lock,
  Network,
  Radar,
  Shield,
  Terminal
} from "lucide-react";
import type { GadgetSpec, GadgetWidget, Session } from "../types";

export function widgetLabel(widget: GadgetWidget) {
  switch (widget) {
    case "top":
      return "Top";
    case "snapshot":
      return "Snapshot";
    case "profile":
      return "Profile";
    case "report":
      return "Report";
    case "trace":
      return "Trace";
    default:
      return "Table";
  }
}

export function iconFor(gadget: GadgetSpec): React.ComponentType<{ size?: string | number }> {
  if (gadget.category === "Network") return Network;
  if (gadget.category === "Security") return Shield;
  if (gadget.category === "Files") return FileSearch;
  if (gadget.category === "Runtime") return Terminal;
  if (gadget.category === "Profile") return Cpu;
  if (gadget.category === "Observability") return Activity;
  if (gadget.kind === "top") return Gauge;
  if (gadget.kind === "snapshot") return HardDrive;
  if (gadget.id.includes("dns") || gadget.id.includes("sni")) return Globe;
  if (gadget.id.includes("ssl") || gadget.id.includes("seccomp")) return Lock;
  return Radar;
}

export function sessionIconFor(session: Session, gadgets: GadgetSpec[]) {
  const gadget = gadgets.find((candidate) => candidate.id === session.gadgetId);
  if (gadget) return iconFor(gadget);
  if (session.gadgetKind === "top") return Gauge;
  if (session.gadgetKind === "snapshot") return HardDrive;
  return Radar;
}
