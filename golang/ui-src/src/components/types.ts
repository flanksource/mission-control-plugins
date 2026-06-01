import type { TargetOption } from "../api";

export type ActiveTab = "dashboard" | "goroutines" | "profiler" | "pprof";
export type SessionStartTarget = TargetOption & { gopsPort?: number; pprofPort?: number };
