"use client";

import {
  IconBrandVscode,
  IconDeviceDesktop,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconGitPullRequest,
  IconHistory,
  IconLayoutGrid,
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
import type { DockviewApi } from "dockview-react";
import { prPanelLabel, prIdentitySlug, prTaskKey } from "@/components/github/pr-utils";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { reviewPanelId } from "@/lib/state/dockview-review-panel-id";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTaskCanvases } from "@/hooks/domains/task/use-task-canvases";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { mrTaskKey } from "@/components/gitlab/mr-detail-panel";
import { RepositoryScriptsMenuItems } from "./repository-scripts-menu";
import { SessionReopenMenuItems } from "./session-reopen-menu";
import { TerminalReopenMenuItems } from "./terminal-reopen-menu";
import { useNormalizedTaskReviews } from "./review-panel-provider";
import { reviewItemId } from "./review-selection";
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
// i18n-exempt: Dockview panel identity prefix, not user-facing copy.
const CANVAS_PANEL_ID_PREFIX = "canvas:";
const DISCOVERABLE_TASK_CANVAS_STATUSES = new Set(["active", "pending", "error"]);

export function isDiscoverableTaskCanvas(canvas: Pick<Canvas, "status">): boolean {
  return DISCOVERABLE_TASK_CANVAS_STATUSES.has(canvas.status);
}

type ReviewMenuIdentity = Pick<ReviewItemSummary, "providerId" | "reviewKey"> &
  Partial<Pick<ReviewItemSummary, "connectionScope" | "repositoryId" | "changeRequestNumber">>;

type ReviewPanelLookup = Pick<DockviewApi, "getPanel">;

/** Reads the params of a live dockview panel, or undefined when no panel with
 * that id is open. */
function panelParams(api: ReviewPanelLookup, id: string): Record<string, unknown> | undefined {
  return api.getPanel(id)?.params as Record<string, unknown> | undefined;
}

/** True when a panel's params identify the built-in GitHub/GitLab review —
 * either through the legacy `prKey`/`mrKey` param or the provider + reviewKey
 * pair. */
function matchesBuiltInReview(
  params: Record<string, unknown> | undefined,
  providerId: "github" | "gitlab",
  reviewKey: string,
  legacyKey: "prKey" | "mrKey",
): boolean {
  return (
    params?.[legacyKey] === reviewKey ||
    ((params?.providerId === providerId || params?.provider === providerId) &&
      params.reviewKey === reviewKey)
  );
}

/** True when a panel's params match a fully-specified registered (non-GitHub/
 * GitLab) review by provider, connection scope, repository, and change-request
 * number. */
function matchesRegisteredReview(
  params: Record<string, unknown> | undefined,
  review: Required<ReviewMenuIdentity>,
): boolean {
  return (
    params?.providerId === review.providerId &&
    params.connectionScope === review.connectionScope &&
    params.repositoryId === review.repositoryId &&
    params.changeRequestNumber !== undefined &&
    String(params.changeRequestNumber) === String(review.changeRequestNumber)
  );
}

/** True only when this exact review already has a canonical or keyed live panel. */
export function isReviewPanelOpen(
  api: ReviewPanelLookup | null,
  review: ReviewMenuIdentity,
): boolean {
  if (!api) return false;
  const canonicalParams = panelParams(api, "pr-detail");
  if (review.providerId === "github") {
    return (
      Boolean(api.getPanel(`pr-detail|${review.reviewKey}`)) ||
      matchesBuiltInReview(canonicalParams, "github", review.reviewKey, "prKey")
    );
  }
  if (review.providerId === "gitlab") {
    return (
      Boolean(api.getPanel(`mr-detail|${review.reviewKey}`)) ||
      matchesBuiltInReview(canonicalParams, "gitlab", review.reviewKey, "mrKey") ||
      matchesBuiltInReview(panelParams(api, "mr-detail"), "gitlab", review.reviewKey, "mrKey")
    );
  }
  const { connectionScope, repositoryId, changeRequestNumber } = review;
  if (
    connectionScope === undefined ||
    repositoryId === undefined ||
    changeRequestNumber === undefined
  ) {
    return false;
  }
  const registeredReview = { ...review, connectionScope, repositoryId, changeRequestNumber };
  return (
    Boolean(api.getPanel(reviewPanelId(registeredReview))) ||
    matchesRegisteredReview(canonicalParams, registeredReview)
  );
}

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
            {`${prPanelLabel(pr.pr_number)} - ${pr.owner}/${pr.repo}`}
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

function TaskCanvasMenuItems({ groupId, taskId }: { groupId: string; taskId: string | null }) {
  const enabled = useFeature("canvases");
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const canvases = useTaskCanvases(taskId, workspaceId, enabled).filter(isDiscoverableTaskCanvas);
  const api = useDockviewStore((s) => s.api);

  if (!enabled || canvases.length === 0) return null;

  return (
    <>
      {canvases.map((canvas) => (
        <DropdownMenuItem
          key={canvas.id}
          onClick={() =>
            api?.addPanel({
              id: `${CANVAS_PANEL_ID_PREFIX}${canvas.id}`,
              component: "canvas",
              title: canvas.title,
              params: { canvasId: canvas.id },
              position: { referenceGroup: groupId },
            })
          }
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-canvas-item-${canvas.id}`}
        >
          <IconLayoutGrid className={MENU_ICON_CLASS} />
          {canvas.title}
        </DropdownMenuItem>
      ))}
    </>
  );
}

/** "+" menu row that opens a prompt-history panel in the given group. */
function PromptHistoryPanelMenuItem({ groupId }: { groupId: string }) {
  const { t } = useTranslation();
  const addPromptHistoryPanel = useDockviewStore((s) => s.addPromptHistoryPanel);
  return (
    <DropdownMenuItem
      data-testid="add-panel-prompt-history-item"
      onClick={() => addPromptHistoryPanel({ groupId })}
      className={MENU_ITEM_CLASS}
    >
      <IconHistory className={MENU_ICON_CLASS} />
      {t("task:promptHistory")}
    </DropdownMenuItem>
  );
}

/** Filters the linked PRs/MRs down to those whose review panel isn't already
 * open, so the "+" menu doesn't offer duplicates. */
function missingBuiltInReviews(
  api: ReviewPanelLookup | null,
  state: Pick<AddPanelMenuState, "prs" | "mrs">,
): Pick<AddPanelMenuState, "prs" | "mrs"> {
  return {
    prs: state.prs.filter(
      (pr) => !isReviewPanelOpen(api, { providerId: "github", reviewKey: prTaskKey(pr) }),
    ),
    mrs: state.mrs.filter(
      (mr) => !isReviewPanelOpen(api, { providerId: "gitlab", reviewKey: mrTaskKey(mr) }),
    ),
  };
}

/** Renders the dockview "+" menu: session/terminal reopen entries, browser,
 * VS Code, plan, port-forwarding toggle, plugin task panels, task canvases,
 * todos, prompt history, changes/files, review panels, and repository scripts. */
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
  const api = useDockviewStore((s) => s.api);
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
      <TaskCanvasMenuItems groupId={groupId} taskId={state.taskId} />
      {!state.isPassthrough && (
        <DropdownMenuItem onClick={() => addTodosPanel({ groupId })} className={MENU_ITEM_CLASS}>
          <IconListCheck className={MENU_ICON_CLASS} />
          {t("common:todos")}
        </DropdownMenuItem>
      )}
      {!state.isPassthrough && <PromptHistoryPanelMenuItem groupId={groupId} />}
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
      <ReviewPanelMenuItems groupId={groupId} state={state} api={api} />
      <RepositoryScriptsMenuItems onRunScript={onRunScript} onRunDevScript={onRunDevScript} />
    </>
  );
}

/** Renders the "+" menu's review rows: linked PRs/MRs (skipping ones already
 * open) plus registered non-GitHub/GitLab review panels. */
function ReviewPanelMenuItems({
  groupId,
  state,
  api,
}: Pick<AddPanelMenuItemsProps, "groupId" | "state"> & { api: ReviewPanelLookup | null }) {
  const { t } = useTranslation();
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);
  const addMRPanel = useDockviewStore((s) => s.addMRPanel);
  const addReviewPanel = useDockviewStore((s) => s.addReviewPanel);
  const { prs, mrs } = missingBuiltInReviews(api, state);
  const reviews = useNormalizedTaskReviews(state.taskId)
    .filter((review) => review.providerId !== "github" && review.providerId !== "gitlab")
    .filter((review) => !isReviewPanelOpen(api, review));
  return (
    <>
      <PRPanelMenuItems prs={prs} onOpenPR={(pr) => addPRPanel(prTaskKey(pr), { groupId })} />
      {mrs.map((mr) => (
        <DropdownMenuItem
          key={mr.id}
          onClick={() => addMRPanel(mrTaskKey(mr), { groupId })}
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-mr-item-${mr.id}`}
        >
          <IconGitPullRequest className={`${MENU_ICON_CLASS} text-orange-500`} />
          {mrs.length > 1
            ? `MR !${mr.mr_iid} - ${mr.project_path}`
            : t("task:mergeRequest", { mriid: mr.mr_iid })}
        </DropdownMenuItem>
      ))}
      {reviews.map((review) => (
        <DropdownMenuItem
          key={reviewItemId(review)}
          onClick={() => addReviewPanel(review, { groupId })}
          className={MENU_ITEM_CLASS}
          data-testid={`add-panel-review-item-${reviewItemId(review)}`}
        >
          <IconGitPullRequest className={MENU_ICON_CLASS} />
          {review.title}
        </DropdownMenuItem>
      ))}
    </>
  );
}
