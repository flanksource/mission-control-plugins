import type { TargetOption } from "../api";

export type ActiveTab = "dashboard" | "goroutines" | "profiler";
export type SessionStartTarget = TargetOption & { useGops?: boolean; usePprof?: boolean; gopsPort?: number; pprofPort?: number };
