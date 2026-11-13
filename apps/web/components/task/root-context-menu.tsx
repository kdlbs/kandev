"use client";

import { Fragment, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArrowUp,
  IconCopy,
  IconExternalLink,
  IconFolder,
  IconGitBranch,
  IconGitPullRequest,
  IconPencil,
  IconTerminal2,
  IconTrash,
} from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useToast } from "@/components/toast-provider";
import { useOpenSessionInEditor } from "@/hooks/use-open-session-in-editor";
import { useOptionalVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { makeUserShellScope } from "@/lib/state/user-shell-scope";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { getTaskPRsForCurrentWorkspace } from "@/hooks/domains/github/use-task-pr-tooltip-hydration";
import type { TaskRepositoryRoot } from "@/hooks/domains/session/use-task-repository-roots";
import { RemoveRootDialog } from "./remove-root-dialog";

export const ROOT_FOCUS_EVENT = "kandev:task-root:focus";
export const ROOT_ACTIVATE_EVENT = "kandev:task-root:activate";

export type RootSurface = "changes" | "files";

export type RootActivationDetail = {
  surface: RootSurface;
  trackerName: string;
  repositoryName: string;
  path: string;
};
type Translate = (key: string, options?: Record<string, unknown>) => string;

type MenuAction = {
  key: string;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
  reason?: string;
  testId?: string;
  onSelect?: () => void | Promise<void>;
  separatorBefore?: boolean;
};

type MenuContext = {
  root: TaskRepositoryRoot;
  linkedPRNumber?: number;
  surface: RootSurface;
  isMobile: boolean;
  copy: (value: string) => void;
  openTerminal: () => void;
  focusOtherSurface: () => void;
  openEditor: () => void;
  openPR: (() => void) | null;
  createPR: (() => void) | null;
  push: () => void;
  canUseGit: boolean;
  remove: () => void;
};

function navigationActions(context: MenuContext, t: Translate): MenuAction[] {
  const { root, surface, isMobile } = context;
  return [
    {
      key: "open-editor",
      label: t("task:rootOpenInEditor"),
      icon: <IconPencil className="h-4 w-4" />,
      disabled: isMobile || !root.worktreeId,
      reason: isMobile ? t("task:rootDisabledPhone") : t("task:rootDisabledNoWorktree"),
      testId: "root-menu-open-editor",
      onSelect: context.openEditor,
    },
    {
      key: "open-terminal",
      label: t("task:rootOpenTerminal"),
      icon: <IconTerminal2 className="h-4 w-4" />,
      disabled: !root.environmentId || !root.taskId,
      reason: !root.environmentId ? t("task:rootDisabledNoSession") : undefined,
      testId: "root-menu-open-terminal",
      onSelect: context.openTerminal,
    },
    {
      key: "show-other",
      label: surface === "changes" ? t("task:rootShowInFiles") : t("task:rootShowInChanges"),
      icon:
        surface === "changes" ? (
          <IconFolder className="h-4 w-4" />
        ) : (
          <IconGitBranch className="h-4 w-4" />
        ),
      testId: "root-menu-show-other",
      onSelect: context.focusOtherSurface,
    },
  ];
}

function copyActions(context: MenuContext, t: Translate): MenuAction[] {
  const { root } = context;
  return [
    {
      key: "copy-full",
      label: t("task:rootCopyFullPath"),
      icon: <IconCopy className="h-4 w-4" />,
      separatorBefore: true,
      testId: "root-menu-copy-full-path",
      onSelect: () => context.copy(root.path),
    },
    {
      key: "copy-relative",
      label: t("task:rootCopyRelativePath"),
      icon: <IconCopy className="h-4 w-4" />,
      testId: "root-menu-copy-relative-path",
      onSelect: () => context.copy(root.relativePath || "."),
    },
    {
      key: "copy-branch",
      label: t("task:rootCopyBranchName"),
      icon: <IconGitBranch className="h-4 w-4" />,
      disabled: !root.branch,
      reason: !root.branch ? t("task:rootDisabledNoBranch") : undefined,
      testId: "root-menu-copy-branch",
      onSelect: () => context.copy(root.branch),
    },
  ];
}

function createPRDisabledReason(context: MenuContext, t: Translate): string | undefined {
  if (!context.canUseGit) return t("task:rootDisabledNoTracker");
  if (!context.createPR) return t("task:rootDisabledNoPR");
  return undefined;
}

function resolveGitScope(root: TaskRepositoryRoot, matched: boolean): string | null | undefined {
  if (matched) return root.trackerName;
  if (root.isPrimary) return undefined;
  return null;
}

function deliveryActions(context: MenuContext, t: Translate): MenuAction[] {
  const prAction: MenuAction = context.openPR
    ? {
        key: "open-pr",
        label: t("task:rootOpenPR", { number: context.linkedPRNumber ?? 0 }),
        icon: <IconGitPullRequest className="h-4 w-4" />,
        testId: "root-menu-open-pr",
        onSelect: context.openPR,
      }
    : {
        key: "create-pr",
        label: t("task:rootCreatePR"),
        icon: <IconExternalLink className="h-4 w-4" />,
        disabled: !context.createPR || !context.canUseGit,
        reason: createPRDisabledReason(context, t),
        testId: "root-menu-create-pr",
        onSelect: context.createPR ?? undefined,
      };
  const actions: MenuAction[] = [
    {
      key: "push",
      label: t("task:rootPush"),
      icon: <IconArrowUp className="h-4 w-4" />,
      separatorBefore: true,
      disabled: !context.canUseGit,
      reason: !context.canUseGit ? t("task:rootDisabledNoTracker") : undefined,
      testId: "root-menu-push",
      onSelect: context.push,
    },
    prAction,
  ];
  if (context.root.removable) {
    actions.push({
      key: "remove",
      label: t("task:rootRemoveFromTask"),
      icon: <IconTrash className="h-4 w-4" />,
      separatorBefore: true,
      testId: "root-menu-remove",
      onSelect: context.remove,
    });
  }
  return actions;
}

function buildMenuActions(context: MenuContext, t: Translate): MenuAction[] {
  return [
    ...navigationActions(context, t),
    ...copyActions(context, t),
    ...deliveryActions(context, t),
  ];
}

function useRootCopy() {
  const { t } = useTranslation();
  const { toast } = useToast();
  return (value: string) => {
    void copyToClipboard(value).then((copied) => {
      toast({
        description: copied ? t("task:rootCopied") : t("task:rootCopyFailed"),
        variant: copied ? "success" : "error",
      });
    });
  };
}

function useRootTerminal(root: TaskRepositoryRoot) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const addUserShell = useAppStore((state) => state.addUserShell);
  return () => {
    if (!root.environmentId || !root.taskId) return;
    void createUserShell(root.environmentId, {
      taskId: root.taskId,
      label: root.displayLabel,
      workingDir: root.path,
    })
      .then((result) => {
        const scope = makeUserShellScope(root.environmentId, root.taskId);
        if (scope) {
          addUserShell(scope, {
            terminalId: result.terminalId,
            kind: result.kind,
            seq: result.seq,
            displayName: result.displayName,
            customName: null,
            state: result.state ?? "open",
            ptyStatus: result.ptyStatus ?? "stopped",
            running: result.ptyStatus === "running",
            label: result.label,
            closable: result.closable ?? true,
            initialCommand: result.initialCommand,
          });
        }
        useDockviewStore
          .getState()
          .addTerminalPanel(
            result.terminalId,
            undefined,
            root.environmentId,
            root.taskId,
            t("common:terminal"),
          );
      })
      .catch((error: unknown) => {
        toast({
          title: t("task:rootTerminalFailed"),
          description: error instanceof Error ? error.message : t("common:requestFailed"),
          variant: "error",
        });
      });
  };
}

function normalizeBranchName(branch: string | null | undefined): string {
  return (branch ?? "")
    .trim()
    .replace(/^refs\/heads\//, "")
    .toLowerCase();
}

function findBranchPR(
  root: TaskRepositoryRoot,
  prs: ReturnType<typeof getTaskPRsForCurrentWorkspace>,
) {
  const branch = normalizeBranchName(root.branch);
  if (!branch) return undefined;
  return (prs ?? []).find((pr) => {
    if (!pr.pr_url || normalizeBranchName(pr.head_branch) !== branch) return false;
    if (pr.repository_id) return pr.repository_id === root.repositoryId;
    return root.isPrimary;
  });
}

function useRootMenuActions(
  root: TaskRepositoryRoot,
  surface: RootSurface,
): {
  items: MenuAction[];
  dialog: ReactNode;
} {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const editor = useOpenSessionInEditor(root.sessionId);
  const dialogs = useOptionalVcsDialogs();
  const git = useSessionGit(root.sessionId);
  const copy = useRootCopy();
  const openTerminal = useRootTerminal(root);
  const [removeOpen, setRemoveOpen] = useState(false);
  const linkedPR = useAppStore((state) =>
    findBranchPR(root, root.taskId ? getTaskPRsForCurrentWorkspace(state, root.taskId) : null),
  );
  const trackerMatched = Boolean(root.trackerName && git.repoNames.includes(root.trackerName));
  const gitScope = resolveGitScope(root, trackerMatched);
  const canUseGit = gitScope !== null;
  const context: MenuContext = {
    root,
    linkedPRNumber: linkedPR?.pr_number,
    surface,
    isMobile,
    copy,
    openTerminal,
    openEditor: () => {
      void editor.open({ worktreeId: root.worktreeId, isDirectory: true });
    },
    focusOtherSurface: () => {
      const target = surface === "changes" ? "files" : "changes";
      const detail: RootActivationDetail = {
        surface: target,
        trackerName: root.trackerName,
        repositoryName: root.repositoryName,
        path: root.relativePath,
      };
      // The mobile layout switches panels itself, then reveals the root after
      // the destination mounts. Desktop dockview panels exist already or mount
      // synchronously, so one animation frame is enough for the listener.
      if (isMobile) {
        window.dispatchEvent(new CustomEvent(ROOT_ACTIVATE_EVENT, { detail }));
        return;
      }
      if (target === "files") {
        useDockviewStore.getState().addFilesPanel();
        window.dispatchEvent(new CustomEvent("switch-to-files-tab"));
      } else {
        useDockviewStore.getState().addChangesPanel();
        window.dispatchEvent(new CustomEvent("switch-to-changes-tab"));
      }
      requestAnimationFrame(() => {
        window.dispatchEvent(new CustomEvent(ROOT_FOCUS_EVENT, { detail }));
      });
    },
    openPR: linkedPR ? () => window.open(linkedPR.pr_url, "_blank", "noopener,noreferrer") : null,
    createPR: dialogs ? () => dialogs.openPRDialog(gitScope ?? undefined) : null,
    push: () => {
      void git.push(undefined, gitScope ?? undefined);
    },
    canUseGit,
    remove: () => setRemoveOpen(true),
  };
  return {
    items: buildMenuActions(context, t),
    dialog: <RemoveRootDialog open={removeOpen} root={root} onOpenChange={setRemoveOpen} />,
  };
}

export function RootContextMenu({
  root,
  surface,
  children,
}: {
  root: TaskRepositoryRoot;
  surface: RootSurface;
  children: ReactNode;
}) {
  const { items, dialog } = useRootMenuActions(root, surface);
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent data-testid="root-context-menu" className="w-64">
        <RootMenuItems items={items} />
      </ContextMenuContent>
      {dialog}
    </ContextMenu>
  );
}

/** Wraps a row in the root menu only when the row maps to a task root. */
export function RootContextMenuIfAvailable({
  root,
  surface,
  children,
}: {
  root?: TaskRepositoryRoot;
  surface: RootSurface;
  children: ReactNode;
}) {
  if (!root) return <>{children}</>;
  return (
    <RootContextMenu root={root} surface={surface}>
      {children}
    </RootContextMenu>
  );
}

/** Action rows shared by the root context menus. */
function RootMenuItems({ items }: { items: MenuAction[] }) {
  return (
    <>
      {items.map((item) => (
        <Fragment key={item.key}>
          {item.separatorBefore && <ContextMenuSeparator />}
          <ContextMenuItem
            data-testid={item.testId}
            disabled={item.disabled}
            className="gap-2"
            onSelect={() => {
              void item.onSelect?.();
            }}
          >
            {item.icon}
            <span className="min-w-0 flex-1 truncate">{item.label}</span>
            {item.disabled && item.reason && (
              <span className="max-w-28 text-right text-[10px] leading-tight text-muted-foreground">
                {item.reason}
              </span>
            )}
          </ContextMenuItem>
        </Fragment>
      ))}
    </>
  );
}
