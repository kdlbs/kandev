"use client";

import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { MRDetailPanelComponent } from "@/components/gitlab/mr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { resolveReviewPanelProvider } from "./review-panel-provider";

export function ReviewDetailPanelComponent({
  panelId,
  params,
}: {
  panelId: string;
  params?: Record<string, unknown>;
}) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const hasGitHubPR = useAppStore((state) => {
    const prs = activeTaskId ? state.taskPRs.byTaskId[activeTaskId] : undefined;
    return Array.isArray(prs) && prs.length > 0;
  });
  const hasGitLabMR = useTaskMRs(activeTaskId).length > 0;
  const provider = resolveReviewPanelProvider(params ?? {}, hasGitHubPR, hasGitLabMR);

  if (provider === "gitlab") {
    return (
      <MRDetailPanelComponent
        panelId={panelId}
        params={{ mrKey: typeof params?.mrKey === "string" ? params.mrKey : undefined }}
      />
    );
  }
  return (
    <PRDetailPanelComponent
      panelId={panelId}
      params={{ prKey: typeof params?.prKey === "string" ? params.prKey : undefined }}
    />
  );
}
