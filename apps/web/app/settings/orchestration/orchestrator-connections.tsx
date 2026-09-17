import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useOrchestratorConversation } from "@/hooks/domains/orchestration/use-orchestrator-conversation";
import { useOrchestrationData } from "@/hooks/domains/orchestration/use-orchestration-data";
import { listOrchestratedTasks } from "@/lib/api/domains/orchestration-api";
export function OpenOrchestratorConversation({
  workspaceId,
  id,
}: {
  workspaceId: string;
  id: string;
}) {
  const { t } = useTranslation();
  const { open, busy } = useOrchestratorConversation(workspaceId, id);
  return (
    <Button disabled={busy} onClick={open}>
      {t("orchestration:openConversation")}
    </Button>
  );
}
export function OrchestratorTasks({ workspaceId, id }: { workspaceId: string; id: string }) {
  const { t } = useTranslation();
  const load = useCallback(() => listOrchestratedTasks(workspaceId, id), [workspaceId, id]);
  const { data, error } = useOrchestrationData(load);
  return (
    <section className="space-y-3" data-testid="orchestrated-tasks">
      <h3 className="font-semibold">{t("orchestration:coordinatedTasks")}</h3>
      <p className="text-sm text-muted-foreground">{t("orchestration:coordinatedTasksHint")}</p>
      {error && <p role="alert">{error}</p>}
      {data?.tasks.length === 0 && (
        <p className="text-sm">{t("orchestration:noCoordinatedTasks")}</p>
      )}
      <ul className="space-y-2">
        {data?.tasks.map((task) => (
          <li key={task.id}>
            <Link
              className="underline"
              href={`/t/${encodeURIComponent(task.id)}?workspaceId=${encodeURIComponent(workspaceId)}`}
            >
              {task.title}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
