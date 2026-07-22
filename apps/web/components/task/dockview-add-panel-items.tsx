"use client";

import {
  IconBrandVscode,
  IconDeviceDesktop,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconGitPullRequest,
} from "@tabler/icons-react";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { prPanelLabel, prIdentitySlug, prTaskKey } from "@/components/github/pr-utils";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { RepositoryScriptsMenuItems } from "./repository-scripts-menu";
import { SessionReopenMenuItems } from "./session-reopen-menu";
import { TerminalReopenMenuItems } from "./terminal-reopen-menu";

type AddPanelMenuState = {
  taskId: string | null;
  isPassthrough: boolean;
  hasChanges: boolean;
  hasFiles: boolean;
  /** All PRs linked to the task; multi-repo tasks render one menu item per PR. */
  prs: TaskPR[];
  mrs: TaskMR[];
};

type AddPanelMenuItemsProps = {
  groupId: string;
  state: AddPanelMenuState;
  onNewSession: () => void;
  onAddTerminal: () => void;
  onRunScript: (scriptId: string) => void;
  onRunDevScript: () => void;
};

export const MENU_ICON_CLASS = "h-3.5 w-3.5 mr-1.5 shrink-0";
export const MENU_ITEM_CLASS = "cursor-pointer text-xs";

export function AddPanelMenuItems({
  groupId,
  state,
  onNewSession,
  onAddTerminal,
  onRunScript,
  onRunDevScript,
}: AddPanelMenuItemsProps) {
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addVscodePanel = useDockviewStore((s) => s.addVscodePanel);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const addFilesPanel = useDockviewStore((s) => s.addFilesPanel);
  const addChangesPanel = useDockviewStore((s) => s.addChangesPanel);
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const addMRPanel = useDockviewStore((s) => s.addMRPanel);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);

  return (
    <>
      {state.taskId && (
        <SessionReopenMenuItems
          taskId={state.taskId}
          groupId={groupId}
          onNewSession={onNewSession}
        />
      )}
      <TerminalReopenMenuItems groupId={groupId} onNewTerminal={onAddTerminal} />
      <DropdownMenuItem
        onClick={() => addBrowserPanel(undefined, groupId)}
        className={MENU_ITEM_CLASS}
      >
        <IconDeviceDesktop className={MENU_ICON_CLASS} />
        Browser
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => addVscodePanel()} className={MENU_ITEM_CLASS}>
        <IconBrandVscode className={MENU_ICON_CLASS} />
        VS Code
      </DropdownMenuItem>
      {!state.isPassthrough && (
        <DropdownMenuItem onClick={() => addPlanPanel({ groupId })} className={MENU_ITEM_CLASS}>
          <IconFileText className={MENU_ICON_CLASS} />
          Plan
        </DropdownMenuItem>
      )}
      {!state.hasChanges && (
        <DropdownMenuItem onClick={() => addChangesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconGitBranch className={MENU_ICON_CLASS} />
          Changes
        </DropdownMenuItem>
      )}
      {!state.hasFiles && (
        <DropdownMenuItem onClick={() => addFilesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconFolder className={MENU_ICON_CLASS} />
          Files
        </DropdownMenuItem>
      )}
      {state.prs.map((pr) => (
        <DropdownMenuItem
          key={pr.id}
          onClick={() => addPRPanel(prTaskKey(pr), activeSessionId)}
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-pr-item-${prIdentitySlug(pr)}`}
        >
          <IconGitPullRequest className={MENU_ICON_CLASS} />
          {state.prs.length > 1
            ? `${prPanelLabel(pr.pr_number)} — ${pr.repo}`
            : prPanelLabel(pr.pr_number)}
        </DropdownMenuItem>
      ))}
      {state.mrs.map((mr) => (
        <DropdownMenuItem
          key={mr.id}
          onClick={() => addMRPanel(mrTaskKey(mr), activeSessionId)}
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-mr-item-${mr.id}`}
        >
          <IconGitPullRequest className={`${MENU_ICON_CLASS} text-orange-500`} />
          {state.mrs.length > 1
            ? `MR !${mr.mr_iid} - ${mr.project_path}`
            : `Merge Request !${mr.mr_iid}`}
        </DropdownMenuItem>
      ))}
      <RepositoryScriptsMenuItems onRunScript={onRunScript} onRunDevScript={onRunDevScript} />
    </>
  );
}
