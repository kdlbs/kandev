"use client";

import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import type { WatchSet } from "@/lib/coordinator/watch-filter";

type WatchesNoneNoticeProps = {
  workspaceId: string;
  coordinatorId: string;
  watchSet: WatchSet | undefined;
  /** Managers get a Choose boards link; readers and the Watches section itself get the sentence only. */
  chooseBoards?: boolean;
  /** On the Configure page: selects the Watches section in place instead of navigating. */
  onChooseBoards?: () => void;
};

export function watchesNoBoard(watchSet: WatchSet | undefined): boolean {
  return (
    watchSet !== undefined && watchSet.scope === "selected" && watchSet.workflowIds.length === 0
  );
}

export function watchesNoProject(watchSet: WatchSet | undefined): boolean {
  const projects = watchSet?.projects;
  return (
    watchSet !== undefined &&
    projects !== undefined &&
    projects.repositoryIds !== null &&
    projects.repositoryIds.length === 0 &&
    !projects.includeNoRepository
  );
}

/** Shown while the coordinator's effective watch set is empty. */
export function WatchesNoneNotice({
  workspaceId,
  coordinatorId,
  watchSet,
  chooseBoards = false,
  onChooseBoards,
}: WatchesNoneNoticeProps) {
  const { t } = useTranslation();
  const noBoard = watchesNoBoard(watchSet);
  if (!noBoard && !watchesNoProject(watchSet)) return null;
  const [sentence, choose] = noBoard
    ? [t("coordinator:watchesNone"), t("coordinator:watchesChooseBoards")]
    : [t("coordinator:watchesNoneProjects"), t("coordinator:watchesChooseProjects")];
  return (
    <p className="text-sm text-muted-foreground" data-testid="watches-none-notice">
      {sentence}{" "}
      {onChooseBoards && (
        <button
          type="button"
          className="cursor-pointer underline"
          onClick={onChooseBoards}
          data-testid="watches-none-choose"
        >
          {choose}
        </button>
      )}
      {chooseBoards && !onChooseBoards && (
        <Link
          href={`/settings/workspaces/${workspaceId}/coordinators/${coordinatorId}?section=watches`}
          className="cursor-pointer underline"
          data-testid="watches-none-choose"
        >
          {choose}
        </Link>
      )}
    </p>
  );
}
