"use client";

import { useCallback, useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { mrTaskKey, selectExplicitPanelMR } from "@/components/gitlab/mr-detail-panel";

export function useMobileMRSelection(
  activeTaskId: string | null,
  effectiveSessionId: string | null,
  requestedPanel: MobileSessionPanel,
  changePanel: (panel: MobileSessionPanel) => void,
  hasGitHubPR: boolean,
) {
  const mrs = useTaskMRs(activeTaskId);
  const reviewSourcesResolved = useAppStore((state) => {
    const workspaceId = state.workspaces.activeId;
    return !!workspaceId && Object.hasOwn(state.taskMRs.byWorkspaceId, workspaceId);
  });
  const reviewMRKey = useAppStore((state) =>
    effectiveSessionId
      ? (state.mobileSession.reviewMRKeyBySessionId?.[effectiveSessionId] ?? null)
      : null,
  );
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const selectedMR = selectExplicitPanelMR(mrs, reviewMRKey);

  useEffect(() => {
    const hasInvalidReviewPreference =
      !hasGitHubPR &&
      requestedPanel === "review" &&
      (!reviewMRKey || (reviewSourcesResolved && !selectedMR));
    if (effectiveSessionId && hasInvalidReviewPreference) {
      setMobileSessionReview(effectiveSessionId, null);
    }
  }, [
    effectiveSessionId,
    requestedPanel,
    hasGitHubPR,
    reviewMRKey,
    reviewSourcesResolved,
    selectedMR,
    setMobileSessionReview,
  ]);

  const handlePanelChange = useCallback(
    (panel: MobileSessionPanel) => {
      if (panel === "review" && effectiveSessionId && !selectedMR) {
        const primaryMR = mrs[0];
        if (primaryMR) setMobileSessionReview(effectiveSessionId, mrTaskKey(primaryMR));
      }
      changePanel(panel);
    },
    [changePanel, effectiveSessionId, mrs, selectedMR, setMobileSessionReview],
  );

  return { mrs, selectedMR, handlePanelChange };
}
