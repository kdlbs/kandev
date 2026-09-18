import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type { Improvement } from "@/lib/api/domains/assistant-maintenance-types";
import { useAssistantPage } from "@/hooks/domains/orchestration/use-assistant";
import { useImprovement } from "@/hooks/domains/orchestration/use-assistant-maintenance";
import { AssistantPanel } from "./assistant-panel";
import { MaintenanceReview } from "./maintenance-review";
export function AssistantImprovements({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const page = useAssistantPage("improvements", binding, revision);
  return (
    <AssistantPanel
      title={t("orchestration:assistantImprovements")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <p className="text-xs text-muted-foreground">{t("orchestration:maintenanceThreshold")}</p>
      <div className="space-y-3">
        {page.entries.map((row) => (
          <ImprovementCard
            key={row.id}
            binding={binding}
            row={row}
            revision={revision}
            refresh={() => void page.refresh()}
          />
        ))}
      </div>
    </AssistantPanel>
  );
}
function ImprovementCard({
  binding,
  row,
  revision,
  refresh,
}: {
  binding: AssistantBinding;
  row: Improvement;
  revision: number;
  refresh: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <article
      className="rounded-md border p-3 space-y-3 min-w-0"
      data-testid="assistant-improvement"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-medium text-sm">
          {t(`orchestration:maintenanceReason_${row.reason}`)}
        </h3>
        <Badge variant="secondary">{t(`orchestration:maintenanceState_${row.state}`)}</Badge>
      </div>
      <p className="text-xs text-muted-foreground">
        {t(`orchestration:maintenanceOrigin_${row.origin}`)} ·{" "}
        {t("orchestration:maintenanceIncidents", { count: row.incident_count })} ·{" "}
        {t("orchestration:maintenanceTasks", { count: row.task_count })}
      </p>
      {row.repair_task_id && (
        <Link
          href={`/t/${encodeURIComponent(row.repair_task_id)}?workspaceId=${encodeURIComponent(row.workspace_id)}`}
          className="inline-flex underline text-sm cursor-pointer max-md:min-h-11 items-center"
        >
          {t("orchestration:maintenanceOpenTask")}
        </Link>
      )}
      <Button
        variant="outline"
        className="cursor-pointer max-md:min-h-11"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {t(open ? "common:close" : "orchestration:maintenanceReview")}
      </Button>
      {open && (
        <ImprovementLoader binding={binding} id={row.id} revision={revision} refresh={refresh} />
      )}
    </article>
  );
}
function ImprovementLoader({
  binding,
  id,
  revision,
  refresh,
}: {
  binding: AssistantBinding;
  id: string;
  revision: number;
  refresh: () => void;
}) {
  const { t } = useTranslation();
  const state = useImprovement(binding, id, revision);
  if (!state.data || state.error)
    return (
      <div role={state.error ? "alert" : "status"}>
        <p>{t(state.error ? "orchestration:assistantUnavailable" : "common:loading")}</p>
        {state.error && (
          <Button
            variant="outline"
            className="cursor-pointer max-md:min-h-11"
            onClick={state.refresh}
          >
            {t("task:retry")}
          </Button>
        )}
      </div>
    );
  return (
    <MaintenanceReview
      binding={binding}
      detail={state.data}
      onUpdated={() => {
        state.refresh();
        refresh();
      }}
    />
  );
}
