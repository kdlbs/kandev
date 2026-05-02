"use client";

import { memo, useCallback, type ReactNode } from "react";
import Link from "next/link";
import { IconBug, IconDots, IconHome, IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@kandev/ui/breadcrumb";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSessionGit } from "@/hooks/domains/session/use-session-git";
import { EditorsMenu } from "@/components/task/editors-menu";
import { BranchPathPopover } from "@/components/task/branch-path-popover";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { DocumentControls } from "@/components/task/document/document-controls";
import { VcsSplitButton } from "@/components/vcs-split-button";
import { PRTopbarButton } from "@/components/github/pr-topbar-button";
import { JiraTicketButton, extractJiraKey } from "@/components/jira/jira-ticket-button";
import { JiraLinkButton } from "@/components/jira/jira-link-button";
import { LinearIssueButton, extractLinearKey } from "@/components/linear/linear-issue-button";
import { LinearLinkButton } from "@/components/linear/linear-link-button";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { WorkflowStepper, type WorkflowStepperStep } from "@/components/task/workflow-stepper";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { QuickChatButton } from "@/components/task/quick-chat-button";
import {
  TopbarActionOverflow,
  type TopbarOverflowItem,
} from "@/components/task/topbar-action-overflow";
import { IntegrationsMenu } from "@/components/integrations/integrations-menu";
import { DEBUG_UI } from "@/lib/config";
import { toast } from "sonner";

type TaskTopBarProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  baseBranch?: string;
  onStartAgent?: (agentProfileId: string) => void;
  onStopAgent?: () => void;
  isAgentRunning?: boolean;
  isAgentLoading?: boolean;
  worktreePath?: string | null;
  worktreeBranch?: string | null;
  repositoryPath?: string | null;
  repositoryName?: string | null;
  /** Total repositories on the task; used to render a "+N" chip on the breadcrumb. */
  repositoryCount?: number;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  workflowSteps?: WorkflowStepperStep[];
  currentStepId?: string | null;
  workflowId?: string | null;
  workspaceId?: string | null;
  isArchived?: boolean;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  remoteExecutorName?: string | null;
  remoteExecutorType?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
};

type TopBarLeftProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  repositoryName?: string | null;
  repositoryCount?: number;
  displayBranch?: string;
  repositoryPath?: string | null;
  worktreePath?: string | null;
  isRemoteExecutor?: boolean;
  remoteExecutorName?: string | null;
  remoteExecutorType?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
  workspaceId?: string | null;
  onRenameBranch?: (newName: string) => Promise<void>;
};

const TaskTopBar = memo(function TaskTopBar({
  taskId,
  activeSessionId,
  taskTitle,
  baseBranch,
  worktreePath,
  worktreeBranch,
  repositoryPath,
  repositoryName,
  repositoryCount,
  showDebugOverlay,
  onToggleDebugOverlay,
  workflowSteps,
  currentStepId,
  workflowId,
  workspaceId,
  isArchived,
  isRemoteExecutor,
  isAgentctlReady,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
}: TaskTopBarProps) {
  const git = useSessionGit(activeSessionId);
  // Prefer live git status branch (updates after rename), fallback to session worktree branch
  const displayBranch = git.branch || worktreeBranch || baseBranch;

  // Callback for renaming branch
  const handleRenameBranch = useCallback(
    async (newName: string) => {
      const result = await git.renameBranch(newName);
      if (result.success) {
        toast.success(`Branch renamed to "${newName}"`);
      } else {
        const msg = result.error || "Failed to rename branch";
        toast.error(msg);
        throw new Error(msg);
      }
    },
    [git],
  );

  return (
    <header
      data-testid="task-topbar"
      className="@container/topbar grid grid-cols-[minmax(0,1fr)_minmax(0,auto)_minmax(0,1fr)] items-center gap-2 overflow-hidden px-3 py-1 border-b border-border"
    >
      <TopBarLeft
        taskId={taskId}
        activeSessionId={activeSessionId}
        taskTitle={taskTitle}
        repositoryName={repositoryName}
        repositoryCount={repositoryCount}
        displayBranch={displayBranch}
        repositoryPath={repositoryPath}
        worktreePath={worktreePath}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorName={remoteExecutorName}
        remoteExecutorType={remoteExecutorType}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
        workspaceId={workspaceId}
        onRenameBranch={activeSessionId ? handleRenameBranch : undefined}
      />
      <div className="min-w-0 justify-self-center overflow-hidden">
        {workflowSteps && workflowSteps.length > 0 && (
          <WorkflowStepper
            steps={workflowSteps}
            currentStepId={currentStepId ?? null}
            taskId={taskId ?? null}
            workflowId={workflowId ?? null}
            isArchived={isArchived}
          />
        )}
      </div>
      <TopBarRight
        taskId={taskId}
        activeSessionId={activeSessionId}
        baseBranch={baseBranch}
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
        workspaceId={workspaceId}
        isRemoteExecutor={isRemoteExecutor}
        isAgentctlReady={isAgentctlReady}
        taskTitle={taskTitle}
      />
    </header>
  );
});

/** Breadcrumb item for the task's repository, with a "+N" chip on multi-repo. */
function RepositoryBreadcrumb({ name, extraCount }: { name: string; extraCount: number }) {
  const tooltipText =
    extraCount > 0 ? `${name} (+${extraCount} more ${extraCount === 1 ? "repo" : "repos"})` : name;
  return (
    <>
      <BreadcrumbSeparator className="shrink-0 @max-[900px]/topbar:hidden" />
      <BreadcrumbItem className="min-w-0 @max-[900px]/topbar:hidden">
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-muted-foreground truncate flex items-center gap-1.5">
              <span className="truncate">{name}</span>
              {extraCount > 0 && (
                <span
                  className="shrink-0 rounded-sm bg-muted px-1 py-px text-[10px] font-medium text-muted-foreground/80"
                  data-testid="topbar-extra-repo-count"
                >
                  +{extraCount}
                </span>
              )}
            </span>
          </TooltipTrigger>
          <TooltipContent>{tooltipText}</TooltipContent>
        </Tooltip>
      </BreadcrumbItem>
    </>
  );
}

// IssueTrackerButtons picks the right ticket button for a task. Jira and
// Linear use the same TEAM-NUMBER identifier shape, so both `extract` calls
// would match "ENG-123" — we resolve ambiguity by preferring whichever
// integration is currently available for the workspace, with Jira winning the
// tie-break since it shipped first. When the title carries no identifier,
// both link buttons are offered (each gated on its own availability).
function IssueTrackerButtons({
  taskId,
  workspaceId,
  taskTitle,
}: {
  taskId: string | null | undefined;
  workspaceId: string | null | undefined;
  taskTitle: string | null | undefined;
}) {
  const jiraAvailable = useJiraAvailable(workspaceId);
  const linearAvailable = useLinearAvailable(workspaceId);
  const jiraKey = extractJiraKey(taskTitle);
  const linearKey = extractLinearKey(taskTitle);

  if (jiraKey && jiraAvailable) {
    return <JiraTicketButton workspaceId={workspaceId} taskTitle={taskTitle} />;
  }
  if (linearKey && linearAvailable) {
    return <LinearIssueButton workspaceId={workspaceId} taskTitle={taskTitle} />;
  }
  return (
    <>
      <JiraLinkButton taskId={taskId} workspaceId={workspaceId} taskTitle={taskTitle} />
      <LinearLinkButton taskId={taskId} workspaceId={workspaceId} taskTitle={taskTitle} />
    </>
  );
}

/** Left section: breadcrumbs, branch pill */
function TopBarLeft({
  taskId,
  activeSessionId,
  taskTitle,
  repositoryName,
  repositoryCount,
  displayBranch,
  repositoryPath,
  worktreePath,
  isRemoteExecutor,
  remoteExecutorName,
  remoteExecutorType,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
  workspaceId,
  onRenameBranch,
}: TopBarLeftProps) {
  const extraRepoCount = (repositoryCount ?? 0) - 1;
  return (
    <div className="flex items-center gap-2.5 min-w-0 overflow-hidden">
      <Breadcrumb className="min-w-0">
        <BreadcrumbList className="flex-nowrap text-sm min-w-0">
          <BreadcrumbItem className="shrink-0">
            <BreadcrumbLink asChild>
              <Link
                href="/"
                className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
              >
                <IconHome className="h-4 w-4" />
              </Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          {repositoryName && (
            <RepositoryBreadcrumb name={repositoryName} extraCount={extraRepoCount} />
          )}
          <BreadcrumbSeparator className="shrink-0" />
          <BreadcrumbItem className="min-w-0">
            <Tooltip>
              <TooltipTrigger asChild>
                <BreadcrumbPage className="font-medium truncate">
                  {taskTitle ?? "Task details"}
                </BreadcrumbPage>
              </TooltipTrigger>
              <TooltipContent>{taskTitle ?? "Task details"}</TooltipContent>
            </Tooltip>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <IntegrationsMenu workspaceId={workspaceId ?? undefined} />

      <div className="shrink-0 @max-[1352px]/topbar:hidden">
        <BranchPathPopover
          displayBranch={displayBranch}
          repositoryPath={repositoryPath}
          worktreePath={worktreePath}
          onRenameBranch={onRenameBranch}
        />
      </div>

      {isRemoteExecutor && (
        <RemoteCloudTooltip
          taskId={taskId ?? ""}
          sessionId={activeSessionId}
          fallbackName={remoteExecutorName ?? remoteExecutorType}
          iconClassName="h-4 w-4"
          status={{
            remote_name: remoteExecutorName ?? undefined,
            remote_state: remoteState ?? undefined,
            remote_created_at: remoteCreatedAt ?? undefined,
            remote_checked_at: remoteCheckedAt ?? undefined,
            remote_status_error: remoteStatusError ?? undefined,
          }}
        />
      )}
    </div>
  );
}

function TopbarCluster({
  label,
  className = "",
  children,
}: {
  label: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      aria-label={label}
      className={`inline-flex shrink-0 items-center gap-1 [&:empty]:hidden ${className}`}
    >
      {children}
    </div>
  );
}

function MoreToolsMenu({
  showDebugOverlay,
  onToggleDebugOverlay,
}: {
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
}) {
  const showDebugItem = DEBUG_UI && onToggleDebugOverlay;
  const debugLabel = showDebugOverlay ? "Hide Debug Info" : "Show Debug Info";

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="h-8 cursor-pointer px-2"
              aria-label="More task tools"
            >
              <IconDots className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>More tools</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuLabel className="text-xs">More tools</DropdownMenuLabel>
        {showDebugItem && (
          <>
            <DropdownMenuItem className="cursor-pointer gap-2" onClick={onToggleDebugOverlay}>
              <IconBug className="h-4 w-4 text-muted-foreground" />
              <span>{debugLabel}</span>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
          </>
        )}
        <DropdownMenuItem asChild className="cursor-pointer gap-2">
          <Link href="/settings/general">
            <IconSettings className="h-4 w-4 text-muted-foreground" />
            <span>Settings</span>
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function SettingsButton() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button asChild size="sm" variant="outline" className="h-8 cursor-pointer px-2">
          <Link href="/settings/general" aria-label="Settings">
            <IconSettings className="h-4 w-4" />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Settings</TooltipContent>
    </Tooltip>
  );
}

function AttentionStatusGroup({
  taskId,
  activeSessionId,
  isArchived,
  workspaceId,
  isRemoteExecutor,
  isAgentctlReady,
  taskTitle,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  isArchived?: boolean;
  workspaceId?: string | null;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  taskTitle?: string;
}) {
  return (
    <TopbarCluster label="Task status and attention" className="[&_button]:h-8 [&_button]:text-xs">
      <DocumentControls activeSessionId={activeSessionId ?? null} />
      {!isArchived && (
        <>
          <PortForwardButton
            isRemoteExecutor={isRemoteExecutor}
            sessionId={activeSessionId}
            isAgentctlReady={isAgentctlReady}
          />
          <PRTopbarButton />
          <IssueTrackerButtons taskId={taskId} workspaceId={workspaceId} taskTitle={taskTitle} />
        </>
      )}
    </TopbarCluster>
  );
}

function TopbarToolsGroup({
  activeSessionId,
  showDebugOverlay,
  onToggleDebugOverlay,
  isArchived,
}: {
  activeSessionId?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
}) {
  const showDebugMenu = DEBUG_UI && onToggleDebugOverlay;

  return (
    <TopbarCluster label="Task tools" className="[&_button]:h-8 [&_button]:text-xs">
      {!isArchived && (
        <>
          <LayoutPresetSelector />
          <EditorsMenu activeSessionId={activeSessionId ?? null} />
        </>
      )}
      {showDebugMenu ? (
        <MoreToolsMenu
          showDebugOverlay={showDebugOverlay}
          onToggleDebugOverlay={onToggleDebugOverlay}
        />
      ) : (
        <SettingsButton />
      )}
    </TopbarCluster>
  );
}

/** Right section: primary VCS action, status/attention, tools menu */
function TopBarRight({
  taskId,
  activeSessionId,
  baseBranch,
  showDebugOverlay,
  onToggleDebugOverlay,
  isArchived,
  workspaceId,
  isRemoteExecutor,
  isAgentctlReady,
  taskTitle,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  baseBranch?: string;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
  workspaceId?: string | null;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  taskTitle?: string;
}) {
  const items: TopbarOverflowItem[] = [];

  if (!isArchived && workspaceId) {
    items.push({
      id: "quick-chat",
      label: "Quick chat",
      priority: 20,
      content: (
        <TopbarCluster label="Quick chat" className="[&_button]:h-8 [&_button]:text-xs">
          <QuickChatButton workspaceId={workspaceId} />
        </TopbarCluster>
      ),
    });
  }

  if (!isArchived) {
    items.push({
      id: "vcs",
      label: "Primary version control action",
      priority: 100,
      required: true,
      content: (
        <TopbarCluster label="Primary version control action">
          <VcsSplitButton
            sessionId={activeSessionId ?? null}
            baseBranch={baseBranch}
            buttonSize="lg"
          />
        </TopbarCluster>
      ),
    });
  }

  items.push({
    id: "attention",
    label: "Task status and attention",
    priority: 80,
    content: (
      <AttentionStatusGroup
        taskId={taskId}
        activeSessionId={activeSessionId}
        isArchived={isArchived}
        workspaceId={workspaceId}
        isRemoteExecutor={isRemoteExecutor}
        isAgentctlReady={isAgentctlReady}
        taskTitle={taskTitle}
      />
    ),
  });

  items.push({
    id: "tools",
    label: "Task tools",
    priority: 10,
    content: (
      <TopbarToolsGroup
        activeSessionId={activeSessionId}
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
      />
    ),
  });

  return (
    <TopbarActionOverflow items={items} className="justify-self-end [&_button]:whitespace-nowrap" />
  );
}

export { TaskTopBar };
