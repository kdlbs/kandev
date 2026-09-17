import { createContext, useContext } from "react";
import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import Link from "@/components/routing/app-link";
import { orchestratorHref } from "@/lib/api/domains/orchestration-api";
import type { Task } from "@/lib/types/http";
export const TaskOrchestrationContext = createContext<Task | null>(null);
export function MobileTaskOrchestratorLink() {
  const task = useContext(TaskOrchestrationContext);
  return <TaskOrchestratorLink task={task} compact />;
}
export function TaskOrchestratorLink({
  task,
  compact = false,
}: {
  task: Task | null;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const enabled = useFeature("orchestration");
  const id = task?.metadata?.orchestration_chief_id;
  return enabled && task && typeof id === "string" ? (
    <Link
      aria-label={t("orchestration:coordinatedBy")}
      className={
        compact
          ? "flex min-h-11 min-w-11 items-center justify-center rounded-md hover:bg-accent"
          : "block px-4 py-2 text-sm underline border-b"
      }
      href={orchestratorHref(task.workspace_id, id)}
    >
      {compact ? <IconSitemap className="size-4" aria-hidden /> : t("orchestration:coordinatedBy")}
    </Link>
  ) : null;
}
