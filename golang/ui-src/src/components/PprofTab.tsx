import { useEffect, useState } from "preact/hooks";
import { FileText } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { pluginURL, type GolangSession } from "../api";
import { GopsRequiredOverlay, LoadingOverlay } from "./ui";

export function PprofTab({ session }: { session: GolangSession }) {
  const url = pluginURL(`pprof/${session.id}/`);
  const [loading, setLoading] = useState(session.pprofAvailable);

  useEffect(() => {
    setLoading(session.pprofAvailable);
  }, [session.id, session.pprofAvailable]);

  const blocked = !session.pprofAvailable || loading;

  return (
    <div className="relative h-full min-h-0">
      {!session.pprofAvailable && <GopsRequiredOverlay>pprof is required for this view.</GopsRequiredOverlay>}
      {loading && <LoadingOverlay>Loading pprof…</LoadingOverlay>}
      <div className={`flex h-full min-h-0 flex-col gap-3 p-4 ${blocked ? "pointer-events-none blur-sm" : ""}`}>
      <div>
        <Button asChild size="sm" variant="outline">
          <a href={url} target="_blank" rel="noreferrer">
            <FileText className="h-4 w-4" />
            Open pprof in new tab
          </a>
        </Button>
      </div>
      {session.pprofAvailable ? (
        <iframe title="pprof" src={url} className="min-h-0 flex-1 rounded-md border bg-background" onLoad={() => setLoading(false)} />
      ) : (
        <div className="min-h-0 flex-1 rounded-md border bg-muted/30" />
      )}
      </div>
    </div>
  );
}
