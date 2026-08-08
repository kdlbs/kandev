"use client";

import {
  memo,
  useEffect,
  useState,
  type ComponentPropsWithoutRef,
  type MouseEvent,
  type Ref,
} from "react";
import Link from "@/components/routing/app-link";
import {
  IconBrandGitlab,
  IconExternalLink,
  IconGitMerge,
  IconPlus,
  IconUnlink,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import {
  useGitLabAvailable,
  useTaskMRs,
  useWorkspaceMRs,
} from "@/hooks/domains/gitlab/use-task-mr";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { deleteTaskMR } from "@/lib/api/domains/gitlab-api";
import type { TaskMR } from "@/lib/types/gitlab";
import type { Repository } from "@/lib/types/http";
import { TaskMRLinkDialog } from "./task-mr-link-dialog";
import { MRAutomationControls } from "./mr-automation-controls";
import { MRCIPopover } from "./mr-ci-popover";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { mrTaskKey } from "./mr-detail-panel";
import { useTranslation } from "react-i18next";
import { MRStatusIcon } from "./mr-task-icon";
import { useHoverPopover } from "@/hooks/domains/github/use-hover-popover";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

const MR_POPOVER_OPEN_DELAY_MS = 150;
const MR_POPOVER_CLOSE_DELAY_MS = 150;

/**
 * Hover-driven popover lifecycle for the single-MR case, mirroring GitHub's
 * PRMultiButton (C7). A multi-MR aggregate popover is out of scope (§9) —
 * hovering a 2+ MR trigger only ever opens the click-driven dropdown.
 */
function useMRPopoverInteractions() {
  const usesTouchDrawer = useTouchDrawer();
  const hover = useHoverPopover({
    openDelayMs: MR_POPOVER_OPEN_DELAY_MS,
    closeDelayMs: MR_POPOVER_CLOSE_DELAY_MS,
    disabled: usesTouchDrawer,
  });
  return { usesTouchDrawer, ...hover };
}

export function mrTriggerClass(compact: boolean, mobile: boolean): string {
  if (mobile) return "h-11 w-11 cursor-pointer";
  return compact ? "h-9 w-9 cursor-pointer" : "cursor-pointer gap-1.5 px-2";
}

export function openMobileMRReview(
  setReview: (sessionId: string, mrKey: string) => void,
  sessionId: string,
  mr: TaskMR,
) {
  setReview(sessionId, mrTaskKey(mr));
}

export function openDesktopMRReview(
  addMRPanel: (mrKey: string) => void,
  mr: TaskMR,
  schedule: (callback: FrameRequestCallback) => number = requestAnimationFrame,
) {
  const mrKey = mrTaskKey(mr);
  addMRPanel(mrKey);
  schedule(() => {
    schedule(() => addMRPanel(mrKey));
  });
}

function MRTriggerContent({
  compact,
  single,
  count,
}: {
  compact: boolean;
  single: TaskMR | null;
  count: number;
}) {
  const { t } = useTranslation();
  if (compact) return <IconBrandGitlab className="h-4 w-4 text-orange-500" />;
  if (single) {
    return (
      <>
        <MRStatusIcon mr={single} className="h-4 w-4" />
        <span className="text-xs font-medium">!{single.mr_iid}</span>
      </>
    );
  }
  return (
    <>
      <IconBrandGitlab className="h-4 w-4 text-orange-500" />
      <span className="text-xs font-medium">
        {count} {t("gitlab:mrs")}
      </span>
    </>
  );
}

// Bare trigger button, deliberately not wrapped in DropdownMenuTrigger here:
// the multi-MR/touch path wraps it in one at the call site, while the
// single-MR desktop path opens the review panel directly on click and needs
// a plain button instead.
function MRMenuTriggerButton({
  single,
  count,
  compact,
  mobile,
  hoverHandlers,
  onClick,
  ref,
  ...triggerProps
}: {
  single: TaskMR | null;
  count: number;
  compact: boolean;
  mobile: boolean;
  hoverHandlers: {
    onTriggerEnter?: () => void;
    onTriggerLeave?: () => void;
  };
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
  // PopoverAnchor's `asChild` clones this element to inject its own anchor
  // ref (React 19 ref-as-prop) so floating-ui can measure the real DOM node.
  // Without forwarding it to Button, the popover never becomes positioned
  // and stays rendered off-screen at Radix's pre-measurement placeholder.
  ref?: Ref<HTMLButtonElement>;
} & ComponentPropsWithoutRef<"button">) {
  const { t } = useTranslation();
  return (
    <Button
      ref={ref}
      {...triggerProps}
      onClick={onClick}
      data-testid="mr-topbar-button"
      data-mr-iid={single?.mr_iid}
      data-mr-state={single?.state}
      size={compact ? "icon-sm" : "sm"}
      variant="outline"
      className={mrTriggerClass(compact, mobile)}
      aria-label={
        single
          ? t("gitlab:gitlabMergeRequest", { mriid: single.mr_iid })
          : t("gitlab:gitlabMergeRequests", { length: count })
      }
      onMouseOver={hoverHandlers.onTriggerEnter}
      onMouseEnter={hoverHandlers.onTriggerEnter}
      onMouseMove={hoverHandlers.onTriggerEnter}
      onPointerOver={hoverHandlers.onTriggerEnter}
      onPointerEnter={hoverHandlers.onTriggerEnter}
      onPointerMove={hoverHandlers.onTriggerEnter}
      onMouseLeave={hoverHandlers.onTriggerLeave}
      onPointerLeave={hoverHandlers.onTriggerLeave}
      onFocus={hoverHandlers.onTriggerEnter}
      onBlur={hoverHandlers.onTriggerLeave}
    >
      <MRTriggerContent compact={compact} single={single} count={count} />
    </Button>
  );
}

function MRDropdownList({
  taskId,
  mrs,
  single,
  canLink,
  onOpenReview,
  onUnlink,
  onLink,
}: {
  taskId: string;
  mrs: TaskMR[];
  single: TaskMR | null;
  canLink: boolean;
  onOpenReview: (mr: TaskMR) => void;
  onUnlink: (associationId: string) => void;
  onLink: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuContent align="end" className="w-72">
      {mrs.map((mr) => (
        <div key={mr.id}>
          <DropdownMenuItem className="cursor-pointer" onSelect={() => onOpenReview(mr)}>
            <IconGitMerge className="h-4 w-4" />
            <span className="min-w-0 truncate">
              {t("gitlab:review")} {mr.project_path}!{mr.mr_iid}
            </span>
          </DropdownMenuItem>
          <DropdownMenuItem asChild className="cursor-pointer">
            <Link href={mr.mr_url} target="_blank" rel="noopener noreferrer">
              <IconExternalLink className="h-4 w-4" /> {t("gitlab:openInGitlab")}
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem
            className="cursor-pointer text-destructive focus:text-destructive"
            onSelect={() => onUnlink(mr.id)}
          >
            <IconUnlink className="h-4 w-4" />
            {t("gitlab:unlink")}
            {mr.mr_iid}
          </DropdownMenuItem>
        </div>
      ))}
      <DropdownMenuSeparator />
      <MRAutomationControls taskId={taskId} mr={single ?? undefined} />
      {canLink ? (
        <>
          <DropdownMenuSeparator />
          <DropdownMenuItem className="cursor-pointer" onSelect={onLink}>
            <IconPlus className="h-4 w-4" />
            {t("gitlab:linkAnotherMergeRequest")}
          </DropdownMenuItem>
        </>
      ) : null}
    </DropdownMenuContent>
  );
}

function useMRDesktopReviewOpener(mobile: boolean) {
  const addMRPanel = useDockviewStore((state) => state.addMRPanel);
  const dockviewReady = useDockviewStore((state) => state.api !== null);
  const isRestoringLayout = useDockviewStore((state) => state.isRestoringLayout);
  // Not restoring right now is necessary but NOT sufficient. Navigating
  // straight to /t/:id runs dockview's `onReady` while the session→env mapping
  // is still hydrating, so it builds a throwaway DEFAULT layout with a null env
  // (see tryRestoreLayout) and `isRestoringLayout` is false the whole time.
  // `switchEnvLayout` only replaces that layout wholesale via `api.fromJSON`
  // once the env arrives, discarding any panel added in the gap — the user
  // clicks the MR button during session load and nothing ever opens. The env
  // being adopted is what marks the layout as actually settled.
  const envAdopted = useDockviewStore((state) => state.currentLayoutEnvId !== null);
  const layoutSettled = envAdopted && !isRestoringLayout;
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setMobileSessionReview = useAppStore((state) => state.setMobileSessionReview);
  const [pendingDesktopMR, setPendingDesktopMR] = useState<TaskMR | null>(null);

  useEffect(() => {
    if (!dockviewReady || !layoutSettled || !pendingDesktopMR) return;
    // Only re-apply once the layout has actually settled: at that point no
    // switchEnvLayout restore can land afterwards and discard this add. The
    // intent is cleared here so a later user-initiated layout change (preset
    // switch, maximize) cannot resurrect a panel the user has since closed.
    openDesktopMRReview(addMRPanel, pendingDesktopMR);
    setPendingDesktopMR(null);
  }, [addMRPanel, dockviewReady, layoutSettled, pendingDesktopMR]);

  return (mr: TaskMR) => {
    if (mobile) {
      if (activeSessionId) openMobileMRReview(setMobileSessionReview, activeSessionId, mr);
      return;
    }
    // Open straight away when we can, so the panel appears on click rather
    // than after the session finishes loading — but never into a layout that
    // is being replaced right now. Retain the intent until dockview exists AND
    // the env layout has settled: `currentLayoutEnvId` outlives `setApi(null)`,
    // so a stale env id can leave `layoutSettled` true while dockview is still
    // remounting, and dropping the intent there would lose the click entirely.
    if (dockviewReady && !isRestoringLayout) openDesktopMRReview(addMRPanel, mr);
    if (!dockviewReady || !layoutSettled) setPendingDesktopMR(mr);
  };
}

function MRMenuButton({
  taskId,
  mrs,
  canLink,
  compact,
  mobile,
  onLink,
  onUnlink,
}: {
  taskId: string;
  mrs: TaskMR[];
  canLink: boolean;
  compact: boolean;
  mobile: boolean;
  onLink: () => void;
  onUnlink: (associationId: string) => void;
}) {
  const single = mrs.length === 1 ? mrs[0] : null;
  const openReview = useMRDesktopReviewOpener(mobile);
  const {
    usesTouchDrawer,
    open: popoverOpen,
    onOpenChange: onPopoverOpenChange,
    onTriggerEnter,
    onTriggerLeave,
    onContentEnter,
    onContentLeave,
  } = useMRPopoverInteractions();

  // Hover preview only makes sense for the single-MR case (MRCIPopover takes
  // one TaskMR); a multi-MR aggregate popover is out of scope (§9). On touch
  // there is no hover at all, so the popover apparatus is skipped entirely —
  // that path keeps the click-driven dropdown unchanged, below.
  const showHoverPreview = single != null && !usesTouchDrawer;

  if (showHoverPreview) {
    // Single MR, desktop: click opens the MR detail panel directly (mirrors
    // GitHub's PRSingleButton) — no intermediate dropdown. Hovering shows a
    // popover with everything else: link/open-in-GitLab/unlink, CI + review
    // summary, automation controls, and a merge action when ready.
    return (
      <Popover open={popoverOpen} onOpenChange={onPopoverOpenChange}>
        <PopoverAnchor asChild>
          <MRMenuTriggerButton
            single={single}
            count={mrs.length}
            compact={compact}
            mobile={mobile}
            hoverHandlers={{ onTriggerEnter, onTriggerLeave }}
            onClick={() => {
              onPopoverOpenChange(false);
              openReview(single);
            }}
          />
        </PopoverAnchor>
        <PopoverContent
          data-testid="mr-topbar-popover"
          align="end"
          sideOffset={4}
          className="w-80"
          onMouseEnter={onContentEnter}
          onMouseMove={onContentEnter}
          onMouseLeave={onContentLeave}
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          <MRCIPopover
            mr={single}
            taskId={taskId}
            enabled={popoverOpen}
            canLink={canLink}
            onOpenDetailPanel={() => openReview(single)}
            onLink={onLink}
            onUnlink={() => onUnlink(single.id)}
          />
        </PopoverContent>
      </Popover>
    );
  }

  return (
    <DropdownMenu>
      <MRMenuTriggerButtonInDropdown
        single={single}
        count={mrs.length}
        compact={compact}
        mobile={mobile}
      />
      <MRDropdownList
        taskId={taskId}
        mrs={mrs}
        single={single}
        canLink={canLink}
        onOpenReview={openReview}
        onUnlink={onUnlink}
        onLink={onLink}
      />
    </DropdownMenu>
  );
}

function MRMenuTriggerButtonInDropdown({
  single,
  count,
  compact,
  mobile,
}: {
  single: TaskMR | null;
  count: number;
  compact: boolean;
  mobile: boolean;
}) {
  return (
    <DropdownMenuTrigger asChild>
      <MRMenuTriggerButton
        single={single}
        count={count}
        compact={compact}
        mobile={mobile}
        hoverHandlers={{}}
      />
    </DropdownMenuTrigger>
  );
}

const EMPTY_REPOSITORIES: Repository[] = [];
const EMPTY_TASK_REPOSITORIES: Array<{ repository_id: string }> = [];

function MRTopbarControl({
  taskId,
  mrs,
  gitlabAvailable,
  compact,
  mobile,
  onLink,
  onUnlink,
}: {
  taskId: string;
  mrs: TaskMR[];
  gitlabAvailable: boolean;
  compact: boolean;
  mobile: boolean;
  onLink: () => void;
  onUnlink: (associationId: string) => void;
}) {
  if (mrs.length > 0) {
    return (
      <MRMenuButton
        taskId={taskId}
        mrs={mrs}
        canLink={gitlabAvailable}
        compact={compact}
        mobile={mobile}
        onLink={onLink}
        onUnlink={onUnlink}
      />
    );
  }
  return null;
}

export const MRTopbarButton = memo(function MRTopbarButton({
  compact = false,
  mobile = false,
}: {
  compact?: boolean;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  const [linkOpen, setLinkOpen] = useState(false);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const repositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const task = useTaskById(activeTaskId);
  useWorkspaceMRs(workspaceId);
  const mrs = useTaskMRs(activeTaskId);
  const gitlabAvailable = useGitLabAvailable();
  const removeTaskMR = useAppStore((state) => state.removeTaskMR);
  const { toast } = useToast();

  if (!activeTaskId || !workspaceId) return null;

  const unlink = async (associationId: string) => {
    try {
      await deleteTaskMR(associationId, workspaceId);
      removeTaskMR(workspaceId, associationId);
    } catch (error) {
      toast({
        title: t("gitlab:failedToUnlinkMergeRequest"),
        description:
          error instanceof Error ? error.message : t("gitlab:theMergeRequestIsStillLinked"),
        variant: "error",
      });
    }
  };

  return (
    <>
      <MRTopbarControl
        taskId={activeTaskId}
        mrs={mrs}
        gitlabAvailable={gitlabAvailable}
        compact={compact}
        mobile={mobile}
        onLink={() => setLinkOpen(true)}
        onUnlink={(associationId) => void unlink(associationId)}
      />
      <TaskMRLinkDialog
        open={linkOpen}
        onOpenChange={setLinkOpen}
        taskId={activeTaskId}
        workspaceId={workspaceId}
        taskRepositories={task?.repositories ?? EMPTY_TASK_REPOSITORIES}
        repositories={repositories}
      />
    </>
  );
});
