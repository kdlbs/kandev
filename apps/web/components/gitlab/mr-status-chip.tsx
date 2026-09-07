"use client";

import { useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import {
  useGitLabAvailable,
  useTaskMRs,
  useUnlinkTaskMR,
  useWorkspaceMRs,
} from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskMRAutomationOptions } from "@/hooks/domains/gitlab/use-task-mr-automation";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { TaskMRLinkDialog } from "./task-mr-link-dialog";
import { mrChipStatus, selectChipMR } from "./mr-task-icon";
import { chipAutomation } from "./mr-status-chip-selection";
import { MRStatusChipPopover } from "./mr-status-chip-popover";
import { MRStatusChipDrawer } from "./mr-status-chip-drawer";
import type { Repository } from "@/lib/types/http";
import type { TaskMR } from "@/lib/types/gitlab";

const EMPTY_REPOSITORIES: Repository[] = [];
const EMPTY_TASK_REPOSITORIES: Array<{ repository_id: string }> = [];

/**
 * Compact GitLab MR status chip for the chat status bar and passthrough
 * toolbar — the GitLab counterpart to PRStatusChip. Renders nothing when
 * the task has no MR whose `state === "open"` (spec: What, Failure modes).
 * `taskId`/`workspaceId` are resolved and narrowed here, before any hook
 * that should only run for a session view with a linked open MR
 * (useTaskMRAutomationOptions, useGitLabAvailable — spec: Sync and
 * freshness, "New trigger 1"/"New trigger 2") is called in the mounted
 * child below.
 */
export function MRStatusChip({ taskId }: { taskId: string | null }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  // Task routes can mount the chip without the task-list shell, which is the
  // usual owner of the workspace MR hydration. Keep the task surface
  // self-sufficient so links created immediately before navigation are not
  // invisible until a full page remount.
  useWorkspaceMRs(workspaceId);
  const mrs = useTaskMRs(taskId);
  const openMRs = Array.isArray(mrs) ? mrs.filter((mr) => mr.state === "open") : [];
  if (!taskId || !workspaceId || openMRs.length === 0) return null;
  return (
    <MRStatusChipMounted taskId={taskId} workspaceId={workspaceId} mrs={mrs} openMRs={openMRs} />
  );
}

function MRStatusChipMounted({
  taskId,
  workspaceId,
  mrs,
  openMRs,
}: {
  taskId: string;
  workspaceId: string;
  mrs: TaskMR[];
  openMRs: TaskMR[];
}) {
  const repositories = useAppStore(
    (state) => state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES,
  );
  const task = useTaskById(taskId);
  const { options: automationOptions } = useTaskMRAutomationOptions(taskId);
  const canLink = useGitLabAvailable();
  const unlink = useUnlinkTaskMR(workspaceId);
  const usesTouchDrawer = useTouchDrawer();
  const [linkOpen, setLinkOpen] = useState(false);

  const liveSelected = selectChipMR(mrs);
  if (!liveSelected) return null;

  const disclosureProps = {
    mrs,
    openCount: openMRs.length,
    liveSelected,
    liveStatus: mrChipStatus(liveSelected),
    automation: chipAutomation(automationOptions, openMRs),
    taskId,
    canLink,
    onUnlink: (associationId: string) => void unlink(associationId),
    onLink: () => setLinkOpen(true),
  };

  return (
    <>
      {usesTouchDrawer ? (
        <MRStatusChipDrawer {...disclosureProps} />
      ) : (
        <MRStatusChipPopover {...disclosureProps} />
      )}
      <TaskMRLinkDialog
        open={linkOpen}
        onOpenChange={setLinkOpen}
        taskId={taskId}
        workspaceId={workspaceId}
        taskRepositories={task?.repositories ?? EMPTY_TASK_REPOSITORIES}
        repositories={repositories}
      />
    </>
  );
}
