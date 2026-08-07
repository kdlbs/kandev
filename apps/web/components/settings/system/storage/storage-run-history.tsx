import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { StorageMaintenanceRun } from "@/lib/types/system";

function dateLabel(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : formatDateTime(parsed);
}

export function StorageRunHistory({
  runs,
  loading = false,
  error,
}: {
  runs: StorageMaintenanceRun[];
  loading?: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  const content = <StorageRunHistoryContent runs={runs} loading={loading} error={error} />;
  return (
    <Card className="min-w-0" data-testid="storage-run-history">
      <CardHeader>
        <CardTitle className="text-base">{t("system:storageMaintenanceHistory")}</CardTitle>
      </CardHeader>
      <CardContent>{content}</CardContent>
    </Card>
  );
}

function StorageRunHistoryContent({
  runs,
  loading,
  error,
}: {
  runs: StorageMaintenanceRun[];
  loading: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" data-testid="storage-run-history-spinner" />
        {t("system:storageHistoryLoading")}
      </div>
    );
  }
  if (error) {
    return (
      <p className="break-words text-sm text-destructive" data-testid="storage-run-history-error">
        {t("system:storageSectionUnavailable")}: {error}
      </p>
    );
  }
  if (runs.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("system:storageHistoryEmpty")}</p>;
  }
  return (
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
  );
}
