import { Badge } from "@flanksource/clicky-ui";
import type { GolangSession } from "../api";
import { portText } from "./utils";

export function SessionSummary({ session }: { session: GolangSession | null }) {
  if (!session) {
    return (
      <div>
        <div className="text-sm font-semibold">No session</div>
        <div className="text-xs text-muted-foreground">Start a session against a ready pod.</div>
      </div>
    );
  }
  return (
    <div className="min-w-0">
      <div className="truncate text-sm font-semibold">{session.pod} / {session.container}</div>
      <div className="mt-1 flex flex-wrap items-center gap-1 text-xs text-muted-foreground">
        <Badge tone={session.gopsAvailable ? "success" : "neutral"} variant="soft" size="sm">
          gops {session.gopsAvailable ? portText(session.gopsRemotePort) : "unavailable"}
        </Badge>
        <Badge tone={session.pprofAvailable ? "success" : "neutral"} variant="soft" size="sm">
          pprof {session.pprofAvailable ? `${portText(session.pprofRemotePort)}${session.pprofBasePath ?? ""}` : "unavailable"}
        </Badge>
        {session.pid ? <Badge variant="outline" size="sm">pid {session.pid}</Badge> : null}
      </div>
    </div>
  );
}
