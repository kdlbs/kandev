import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useAssistantPage } from "@/hooks/domains/orchestration/use-assistant";
import { AssistantPanel } from "./assistant-panel";
export function AssistantObjectives({
  binding,
  revision,
}: {
  binding: AssistantBinding;
  revision: number;
}) {
  const { t } = useTranslation();
  const page = useAssistantPage("objectives", binding, revision);
  return (
    <AssistantPanel
      title={t("orchestration:assistantObjectives")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <div className="space-y-3">
        {page.entries.map((goal) => (
          <article key={goal.id} className="rounded-md border p-3 space-y-2">
            <h3 className="font-medium break-words">{goal.title}</h3>
            <p className="text-xs text-muted-foreground">
              {t(`orchestration:assistantGoal_${goal.status}`)} ·{" "}
              {t(`orchestration:assistantMode_${goal.mode}`)}
            </p>
            <ul className="space-y-2">
              {(goal.acceptance ?? []).map((criterion) => (
                <li key={criterion.id} className="text-sm">
                  <p>{criterion.description}</p>
                  <div className="space-y-1">
                    {(goal.evidence ?? [])
                      .filter(
                        (e) =>
                          e.criterion_id === criterion.id &&
                          e.acceptance_revision === goal.acceptance_revision,
                      )
                      .map((e, index) =>
                        e.task_id ? (
                          <Link
                            key={index}
                            className="inline-flex underline cursor-pointer text-xs max-md:min-h-11 items-center"
                            href={`/tasks/${encodeURIComponent(e.task_id)}?sessionId=${encodeURIComponent(e.session_id ?? "")}`}
                          >
                            {t("orchestration:assistantEvidence")}
                          </Link>
                        ) : (
                          <p key={index} className="text-xs text-muted-foreground">
                            {t("orchestration:assistantConversationEvidence")}
                          </p>
                        ),
                      )}
                  </div>
                </li>
              ))}
            </ul>
          </article>
        ))}
      </div>
    </AssistantPanel>
  );
}
