import { FileText } from "lucide-react";
import { Button } from "@flanksource/clicky-ui";
import { pluginURL, type GolangSession } from "../api";
import { Empty } from "./ui";

export function PprofTab({ session }: { session: GolangSession }) {
  if (!session.pprofAvailable) return <Empty>Pprof is not available for this session.</Empty>;
  const url = pluginURL(`pprof/${session.id}/`);
  return (
    <div className="flex h-full min-h-0 flex-col gap-3 p-4">
      <div>
        <Button asChild size="sm" variant="outline">
          <a href={url} target="_blank" rel="noreferrer">
            <FileText className="h-4 w-4" />
            Open pprof index
          </a>
        </Button>
      </div>
      <iframe title="pprof" src={url} className="min-h-0 flex-1 rounded-md border bg-background" />
    </div>
  );
}
