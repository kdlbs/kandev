"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { TaskPR, WorkflowAttention } from "@/lib/types/github";
import {
  getWorkflowAttentionForDisplay,
  isWorkflowApprovalRequired,
  workflowAttentionReasonKey,
} from "./pr-workflow-attention";

export function PRWorkflowAttentionNotice({
  pr,
  attention: suppliedAttention,
}: {
  pr: TaskPR;
  attention?: WorkflowAttention | null;
}) {
  const { t } = useTranslation();
  const attention = getWorkflowAttentionForDisplay(pr, suppliedAttention);
  if (!attention) return null;

  return (
    <section
      data-testid="pr-workflow-attention"
      role="status"
      className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs"
    >
      <div className="flex items-start gap-2">
        <IconAlertTriangle
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1 space-y-2">
          <p className="font-medium text-foreground">
            {attention.state === "unknown"
              ? t("github:workflowStatusUnavailable")
              : t(
                  isWorkflowApprovalRequired(attention)
                    ? "github:workflowAwaitingApproval"
                    : "github:workflowNeedsAttention",
                )}
          </p>
          {attention.runs.map((run) => (
            <div
              key={`${run.run_id}-${run.run_attempt}`}
              data-testid="pr-workflow-attention-run"
              className="flex flex-col gap-1"
            >
              <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
                <span className="min-w-0 break-words font-medium text-foreground">
                  {run.name || t("github:workflow")}
                </span>
                <span className="text-muted-foreground">
                  {t(workflowAttentionReasonKey(run.reason))}
                </span>
              </div>
              {run.url ? (
                <a
                  data-testid="pr-workflow-attention-link"
                  href={run.url}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex min-h-11 items-center self-start text-primary hover:underline sm:min-h-0"
                >
                  {t("github:viewWorkflowOnGithub")}
                </a>
              ) : null}
            </div>
          ))}
          {attention.stale ? (
            <p className="text-muted-foreground">{t("github:workflowAttentionStale")}</p>
          ) : null}
        </div>
      </div>
    </section>
  );
}
