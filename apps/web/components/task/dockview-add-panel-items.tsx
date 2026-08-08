"use client";

import {
  IconBrandVscode,
  IconDeviceDesktop,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconGitPullRequest,
  IconListCheck,
  IconNetwork,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import { prPanelLabel, prIdentitySlug, prTaskKey } from "@/components/github/pr-utils";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { RepositoryScriptsMenuItems } from "./repository-scripts-menu";
import { SessionReopenMenuItems } from "./session-reopen-menu";
import { TerminalReopenMenuItems } from "./terminal-reopen-menu";
import type { PortForwardingVisibility } from "./port-forwarding-visibility-provider";

export type AddPanelMenuState = {
  taskId: string | null;
  isPassthrough: boolean;
  hasChanges: boolean;
  hasFiles: boolean;
  /** All PRs linked to the task; multi-repo tasks render one menu item per PR. */
  prs: TaskPR[];
  mrs: TaskMR[];
  portForwarding?: PortForwardingVisibility;
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

const PR_SUBMENU_TEST_ID = "add-panel-pr-submenu";

/**
 * Linked GitHub PR entries for the dockview "+" menu. A single PR renders as
 * one inline row; multiple PRs collapse behind a "Pull requests" sub-menu so
 * tasks with up to ten linked PRs don't stretch the main menu too tall.
 */
function PRPanelMenuItems({ prs, onOpenPR }: { prs: TaskPR[]; onOpenPR: (pr: TaskPR) => void }) {
  const { t } = useTranslation();
  if (prs.length === 0) return null;
  if (prs.length === 1) {
    const pr = prs[0];
    return (
      <DropdownMenuItem
        onClick={() => onOpenPR(pr)}
        className={MENU_ITEM_CLASS}
        data-testid={`add-panel-pr-item-${prIdentitySlug(pr)}`}
      >
        <IconGitPullRequest className={MENU_ICON_CLASS} />
        {prPanelLabel(pr.pr_number)}
      </DropdownMenuItem>
    );
  }
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        className={MENU_ITEM_CLASS}
        data-testid={PR_SUBMENU_TEST_ID}
        data-pr-count={prs.length}
      >
        <IconGitPullRequest className={MENU_ICON_CLASS} />
        {t("task:pullRequests")}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="max-h-[min(24rem,60vh)] w-52 overflow-y-auto">
        {prs.map((pr) => (
          <DropdownMenuItem
            key={pr.id}
            onClick={() => onOpenPR(pr)}
            className={MENU_ITEM_CLASS}
            data-testid={`add-panel-pr-item-${prIdentitySlug(pr)}`}
          >
            <IconGitPullRequest className={MENU_ICON_CLASS} />
            {`${prPanelLabel(pr.pr_number)} — ${pr.owner}/${pr.repo}`}
          </DropdownMenuItem>
        ))}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

/** One "+" menu row per plugin-registered task panel (AC1), rendered after Plan. */
function PluginTaskPanelMenuItems({ groupId }: { groupId: string }) {
  usePluginRegistry();
  const addPluginPanel = useDockviewStore((s) => s.addPluginPanel);
  const panels = pluginRegistry.getTaskPanels();

  return (
    <>
      {panels.map((registration) => {
        const Icon = resolvePluginIcon(registration.icon);
        return (
          <DropdownMenuItem
            key={`${registration.pluginId}:${registration.id}`}
            onClick={() =>
              addPluginPanel(registration.pluginId, registration.id, registration.title, {
                groupId,
              })
            }
            className={MENU_ITEM_CLASS}
            data-testid={`add-panel-plugin-item-${registration.pluginId}-${registration.id}`}
          >
            <Icon className={MENU_ICON_CLASS} />
            {registration.title}
          </DropdownMenuItem>
        );
      })}
    </>
  );
}

export function AddPanelMenuItems({
  groupId,
  state,
  onNewSession,
  onAddTerminal,
  onRunScript,
  onRunDevScript,
}: AddPanelMenuItemsProps) {
  const { t } = useTranslation();
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addVscodePanel = useDockviewStore((s) => s.addVscodePanel);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const addTodosPanel = useDockviewStore((s) => s.addTodosPanel);
  const addFilesPanel = useDockviewStore((s) => s.addFilesPanel);
  const addChangesPanel = useDockviewStore((s) => s.addChangesPanel);
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const addMRPanel = useDockviewStore((s) => s.addMRPanel);

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
        {t("task:browser")}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => addVscodePanel()} className={MENU_ITEM_CLASS}>
        <IconBrandVscode className={MENU_ICON_CLASS} />
        {t("task:vsCode")}
      </DropdownMenuItem>
      {!state.isPassthrough && (
        <DropdownMenuItem onClick={() => addPlanPanel({ groupId })} className={MENU_ITEM_CLASS}>
          <IconFileText className={MENU_ICON_CLASS} />
          {t("task:plan")}
        </DropdownMenuItem>
      )}
      {state.portForwarding && (
        <DropdownMenuCheckboxItem
          checked={state.portForwarding.enabled}
          disabled={!state.portForwarding.canToggle || state.portForwarding.isUpdating}
          onCheckedChange={() =>
            void state.portForwarding?.togglePortForwarding({ openDialogOnEnable: true })
          }
          className={MENU_ITEM_CLASS}
          data-testid="port-forwarding-menu-item"
        >
          <IconNetwork className={MENU_ICON_CLASS} />
          {t("task:portForwarding")}
        </DropdownMenuCheckboxItem>
      )}
      <PluginTaskPanelMenuItems groupId={groupId} />
      {!state.isPassthrough && (
        <DropdownMenuItem onClick={() => addTodosPanel({ groupId })} className={MENU_ITEM_CLASS}>
          <IconListCheck className={MENU_ICON_CLASS} />
          {t("common:todos")}
        </DropdownMenuItem>
      )}
      {!state.hasChanges && (
        <DropdownMenuItem onClick={() => addChangesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconGitBranch className={MENU_ICON_CLASS} />
          {t("task:changes")}
        </DropdownMenuItem>
      )}
      {!state.hasFiles && (
        <DropdownMenuItem onClick={() => addFilesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconFolder className={MENU_ICON_CLASS} />
          {t("task:files")}
        </DropdownMenuItem>
      )}
      <PRPanelMenuItems prs={state.prs} onOpenPR={(pr) => addPRPanel(prTaskKey(pr))} />
      {state.mrs.map((mr) => (
        <DropdownMenuItem
          key={mr.id}
          onClick={() => addMRPanel(mrTaskKey(mr))}
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-mr-item-${mr.id}`}
        >
          <IconGitPullRequest className={`${MENU_ICON_CLASS} text-orange-500`} />
          {state.mrs.length > 1
            ? `MR !${mr.mr_iid} - ${mr.project_path}`
            : t("task:mergeRequest", { mriid: mr.mr_iid })}
        </DropdownMenuItem>
      ))}
      <RepositoryScriptsMenuItems onRunScript={onRunScript} onRunDevScript={onRunDevScript} />
    </>
  );
}
