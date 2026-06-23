"use client";

import { memo, type ReactNode } from "react";
import Link from "@/components/routing/app-link";
import { IconBug, IconCircleDot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage } from "@kandev/ui/breadcrumb";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { EditorsMenu } from "@/components/task/editors-menu";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { DocumentControls } from "@/components/task/document/document-controls";
import { PRTopbarButton } from "@/components/github/pr-topbar-button";
import { MRTopbarButton } from "@/components/gitlab/mr-topbar-button";
import { JiraTicketButton, extractJiraKey } from "@/components/jira/jira-ticket-button";
import { JiraLinkButton } from "@/components/jira/jira-link-button";
import { LinearIssueButton, extractLinearKey } from "@/components/linear/linear-issue-button";
import { LinearLinkButton } from "@/components/linear/linear-link-button";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { ExecutorSettingsButton } from "@/components/task/executor-settings-button";
import { WorkflowStepper, type WorkflowStepperStep } from "@/components/task/workflow-stepper";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import { isDebugUI } from "@/lib/config";

type TaskTopBarProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  onStartAgent?: (agentProfileId: string) => void;
  onStopAgent?: () => void;
  isAgentRunning?: boolean;
  isAgentLoading?: boolean;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  workflowSteps?: WorkflowStepperStep[];
  currentStepId?: string | null;
  workflowId?: string | null;
  workspaceId?: string | null;
  issueUrl?: string;
  issueNumber?: number;
  isArchived?: boolean;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  remoteExecutorType?: string | null;
  officeTaskHref?: string | null;
};

type TopBarLeftProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  remoteExecutorType?: string | null;
  isArchived?: boolean;
};

const TaskTopBar = memo(function TaskTopBar({
  taskId,
  activeSessionId,
  taskTitle,
  showDebugOverlay,
  onToggleDebugOverlay,
  workflowSteps,
  currentStepId,
  workflowId,
  workspaceId,
  isArchived,
  isRemoteExecutor,
  isAgentctlReady,
  issueUrl,
  issueNumber,
  remoteExecutorType,
  officeTaskHref,
}: TaskTopBarProps) {
  return (
    <header
      data-testid="task-topbar"
      className="@container/topbar grid h-10 shrink-0 grid-cols-[minmax(0,auto)_minmax(0,1fr)_auto] items-center gap-2 overflow-hidden px-3 py-1 border-b border-border"
    >
      <TopBarLeft
        taskId={taskId}
        activeSessionId={activeSessionId}
        taskTitle={taskTitle}
        remoteExecutorType={remoteExecutorType}
        isArchived={isArchived}
      />
      <div className="min-w-0 justify-self-stretch overflow-hidden">
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
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
        workspaceId={workspaceId}
        isRemoteExecutor={isRemoteExecutor}
        isAgentctlReady={isAgentctlReady}
        taskTitle={taskTitle}
        issueUrl={issueUrl}
        issueNumber={issueNumber}
        officeTaskHref={officeTaskHref}
      />
    </header>
  );
});

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
  const jiraAvailable = useJiraAvailable();
  const linearAvailable = useLinearAvailable();
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

/** Left section: task name breadcrumb + executor info. Home + integrations
 *  moved to the unified AppSidebar in the UI overhaul. */
function TopBarLeft({
  taskId,
  activeSessionId,
  taskTitle,
  remoteExecutorType,
  isArchived,
}: TopBarLeftProps) {
  const showExecutorSettings = shouldShowExecutorEnvironmentControls(remoteExecutorType);
  return (
    <div className="flex items-center gap-2.5 min-w-0 overflow-hidden">
      <Breadcrumb className="min-w-0">
        <BreadcrumbList className="flex-nowrap text-sm min-w-0">
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

      {!isArchived && showExecutorSettings && (
        <ExecutorSettingsButton taskId={taskId} sessionId={activeSessionId ?? null} />
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

function DebugOverlayToggle({
  showDebugOverlay,
  onToggleDebugOverlay,
}: {
  showDebugOverlay?: boolean;
  onToggleDebugOverlay: () => void;
}) {
  const label = showDebugOverlay ? "Hide Debug Info" : "Show Debug Info";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-7 cursor-pointer px-2"
          onClick={onToggleDebugOverlay}
          aria-label={label}
        >
          <IconBug className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
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
  issueUrl,
  issueNumber,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  isArchived?: boolean;
  workspaceId?: string | null;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  taskTitle?: string;
  issueUrl?: string;
  issueNumber?: number;
}) {
  return (
    <TopbarCluster label="Task status and attention" className="[&_button]:h-7 [&_button]:text-xs">
      <DocumentControls activeSessionId={activeSessionId ?? null} />
      {!isArchived && (
        <>
          <PortForwardButton
            isRemoteExecutor={isRemoteExecutor}
            sessionId={activeSessionId}
            isAgentctlReady={isAgentctlReady}
          />
          {/* PR (GitHub) and MR (GitLab) buttons each render nothing when no
              rows match, so showing both covers GitHub-only, GitLab-only, and
              multi-repo tasks without needing an explicit provider switch. */}
          <GitHubIssueTopbarButton issueUrl={issueUrl} issueNumber={issueNumber} />
          <PRTopbarButton />
          <MRTopbarButton />
          <IssueTrackerButtons taskId={taskId} workspaceId={workspaceId} taskTitle={taskTitle} />
        </>
      )}
    </TopbarCluster>
  );
}

function GitHubIssueTopbarButton({
  issueUrl,
  issueNumber,
}: {
  issueUrl?: string;
  issueNumber?: number;
}) {
  if (!issueUrl) return null;
  const label = issueNumber ? `#${issueNumber}` : "Issue";
  const tooltip = issueNumber ? `GitHub issue #${issueNumber}` : "GitHub issue";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          data-testid="issue-topbar-button"
          data-issue-number={issueNumber}
          size="sm"
          variant="outline"
          className="h-7 cursor-pointer gap-1.5 px-2"
        >
          <Link href={issueUrl} target="_blank" rel="noopener noreferrer">
            <IconCircleDot className="h-4 w-4 text-green-500" />
            <span className="text-xs font-medium">{label}</span>
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
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
  const showDebugToggle = isDebugUI() && onToggleDebugOverlay;

  return (
    <TopbarCluster label="Task tools" className="[&_button]:h-7 [&_button]:text-xs">
      {!isArchived && (
        <>
          <LayoutPresetSelector />
          <EditorsMenu activeSessionId={activeSessionId ?? null} />
        </>
      )}
      {showDebugToggle && (
        <DebugOverlayToggle
          showDebugOverlay={showDebugOverlay}
          onToggleDebugOverlay={onToggleDebugOverlay}
        />
      )}
    </TopbarCluster>
  );
}

/** Right section: status/attention + tools rendered inline.
 *  The former overflow popover was removed in the UI overhaul — every cluster
 *  is always visible so users don't have to discover the dots menu. */
function TopBarRight({
  taskId,
  activeSessionId,
  showDebugOverlay,
  onToggleDebugOverlay,
  isArchived,
  workspaceId,
  isRemoteExecutor,
  isAgentctlReady,
  taskTitle,
  issueUrl,
  issueNumber,
  officeTaskHref,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
  workspaceId?: string | null;
  isRemoteExecutor?: boolean;
  isAgentctlReady?: boolean;
  taskTitle?: string;
  issueUrl?: string;
  issueNumber?: number;
  officeTaskHref?: string | null;
}) {
  return (
    <div className="flex items-center justify-self-end gap-2 [&_button]:whitespace-nowrap">
      <TopbarMetrics activeSessionId={activeSessionId} />
      {officeTaskHref && (
        <TopbarCluster label="Open in office view" className="[&_a]:h-7 [&_a]:text-xs">
          <Button asChild size="sm" variant="outline" className="h-7 cursor-pointer px-2">
            <Link href={officeTaskHref}>Open in office view</Link>
          </Button>
        </TopbarCluster>
      )}
      <AttentionStatusGroup
        taskId={taskId}
        activeSessionId={activeSessionId}
        isArchived={isArchived}
        workspaceId={workspaceId}
        isRemoteExecutor={isRemoteExecutor}
        isAgentctlReady={isAgentctlReady}
        taskTitle={taskTitle}
        issueUrl={issueUrl}
        issueNumber={issueNumber}
      />
      <TopbarToolsGroup
        activeSessionId={activeSessionId}
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
      />
    </div>
  );
}

function shouldShowExecutorEnvironmentControls(executorType?: string | null): boolean {
  switch (executorType) {
    case "local_docker":
    case "remote_docker":
    case "sprites":
    case "ssh":
      return true;
    default:
      return false;
  }
}

export { TaskTopBar };
