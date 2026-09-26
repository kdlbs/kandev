"use client";

import { IconGitMerge } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useMRActions } from "@/hooks/domains/gitlab/use-mr-actions";
import { mergeMR } from "@/lib/api/domains/gitlab-api";
import type { TaskMR } from "@/lib/types/gitlab";
import { isMRReadyToMerge } from "./mr-task-icon";

/**
 * Compact "Merge" action for the MR hover popover — mirrors GitHub's
 * PRMergeButton compact variant (immediate merge on click, no confirmation
 * dialog). Renders nothing unless the MR is fully ready, so the readiness
 * gate — not a confirmation step — is what keeps this safe.
 */
export function MRMergeButton({ mr }: { mr: TaskMR }) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { pendingAction, run } = useMRActions(() => {});

  if (!isMRReadyToMerge(mr)) return null;
  const merging = pendingAction === "merge";

  return (
    <button
      type="button"
      data-testid="mr-merge-button"
      disabled={merging || !workspaceId}
      onClick={(e) => {
        e.stopPropagation();
        if (!workspaceId) return;
        void run("merge", () =>
          mergeMR({
            workspaceId,
            project: mr.project_path,
            iid: mr.mr_iid,
            host: mr.host,
            squash: false,
          }),
        );
      }}
      className="inline-flex w-full cursor-pointer items-center justify-center gap-1.5 rounded-md bg-green-600 px-2 py-1.5 text-xs font-medium text-white shadow-sm transition-colors hover:bg-green-700 disabled:cursor-default disabled:opacity-60"
    >
      <IconGitMerge className="h-3.5 w-3.5" />
      {merging ? t("gitlab:merging") : t("gitlab:merge")}
    </button>
  );
}
