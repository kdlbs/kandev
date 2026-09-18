import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import { useAssistantPage } from "@/hooks/domains/orchestration/use-assistant";
import { AssistantPanel } from "./assistant-panel";
import { AttentionCard } from "./attention-card";
export function AssistantAttentionPanel({
  binding,
  revision,
  onUpdated,
}: {
  binding: AssistantBinding;
  revision: number;
  onUpdated: () => void;
}) {
  const { t } = useTranslation();
  const page = useAssistantPage("attention", binding, revision);
  const [open, setOpen] = useState<string>();
  return (
    <AssistantPanel
      title={t("orchestration:assistantAttention")}
      {...page}
      count={page.entries.length}
      onRefresh={() => void page.refresh()}
      onMore={() => void page.loadMore()}
    >
      <div className="space-y-3">
        {page.entries.map((row) =>
          open === row.id ? (
            <AttentionCard
              key={`${row.id}:${row.revision}`}
              binding={binding}
              row={row}
              onResolved={() => {
                void page.refresh();
                onUpdated();
              }}
              onDismiss={() => setOpen(undefined)}
            />
          ) : (
            <article
              key={row.id}
              className="rounded-md border p-3 space-y-2"
              data-testid="assistant-attention-row"
            >
              <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                <span>{t(`orchestration:assistantAttention_${row.kind}`)}</span>
                <span>{t(`orchestration:assistantAttentionState_${row.state}`)}</span>
              </div>
              <p className="text-sm break-words">{row.summary}</p>
              {row.state === "pending" && (row.kind === "question" || row.kind === "permission") ? (
                <Button
                  variant="outline"
                  disabled={Boolean(page.error)}
                  className="cursor-pointer max-md:min-h-11"
                  onClick={() => setOpen(row.id)}
                >
                  {t("orchestration:assistantReviewInput")}
                </Button>
              ) : (
                <Link
                  href={`/tasks/${encodeURIComponent(row.task_id)}?sessionId=${encodeURIComponent(row.session_id)}`}
                  className="inline-flex items-center text-sm underline cursor-pointer max-md:min-h-11"
                >
                  {t("orchestration:assistantOpenTask")}
                </Link>
              )}
            </article>
          ),
        )}
      </div>
    </AssistantPanel>
  );
}
