import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { StorageMaintenanceRun } from "@/lib/types/system";

function dateLabel(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

export function StorageRunHistory({ runs }: { runs: StorageMaintenanceRun[] }) {
  return (
    <Card className="min-w-0" data-testid="storage-run-history">
      <CardHeader>
        <CardTitle className="text-base">Maintenance history</CardTitle>
      </CardHeader>
      <CardContent>
        {runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No storage maintenance runs yet.</p>
        ) : (
          <Accordion type="multiple">
            {runs.map((run) => (
              <AccordionItem key={run.id} value={run.id} data-testid={`storage-run-${run.id}`}>
                <AccordionTrigger className="min-h-11 items-center px-3 no-underline">
                  <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                    <Badge variant={run.state === "failed" ? "destructive" : "outline"}>
                      {run.state}
                    </Badge>
                    <span className="capitalize">{run.trigger}</span>
                    <span className="text-muted-foreground">{dateLabel(run.started_at)}</span>
                  </span>
                </AccordionTrigger>
                <AccordionContent className="px-3">
                  {run.message && <p className="mb-2 break-words text-amber-600">{run.message}</p>}
                  <pre className="max-w-full overflow-hidden whitespace-pre-wrap break-all rounded bg-muted p-3 text-[11px]">
                    {JSON.stringify(run.result, null, 2)}
                  </pre>
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        )}
      </CardContent>
    </Card>
  );
}
