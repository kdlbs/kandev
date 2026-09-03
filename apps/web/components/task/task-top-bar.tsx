"use client";

import { memo, type ReactNode } from "react";
import Link from "@/components/routing/app-link";
import { IconBug, IconCircleDot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PageTopbar, type ParentCrumb } from "@/components/page-topbar";
import { useOfficeProject } from "@/hooks/use-office-workspace-data";
import { TaskTopBarTitle } from "@/components/task/task-top-bar-title";
import { EditorsMenu } from "@/components/task/editors-menu";
import { LayoutPresetSelector } from "@/components/task/layout-preset-selector";
import { DocumentControls } from "@/components/task/document/document-controls";
import { PRTopbarButton } from "@/components/github/pr-topbar-button";
import { MRTopbarButton } from "@/components/gitlab/mr-topbar-button";
import { JiraTicketButton, extractJiraKey } from "@/components/jira/jira-ticket-button";
import { LinearIssueButton, extractLinearKey } from "@/components/linear/linear-issue-button";
import { useJiraAvailable } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { PortForwardButton } from "@/components/task/port-forward-dialog";
import { ExecutorSettingsButton } from "@/components/task/executor-settings-button";
import { TaskUnarchiveButton } from "@/components/task/task-unarchive-button";
import { WorkflowStepper, type WorkflowStepperStep } from "@/components/task/workflow-stepper";
import { TaskTopBarPluginActions } from "@/components/task/task-top-bar-plugin-actions";
import { TopbarMetrics } from "@/components/system-metrics/topbar-metrics";
import { RegisteredChangeRequestStatus } from "@/components/integrations/registered-change-request-status";
import { isDebugUI } from "@/lib/config";
import { useTranslation } from "react-i18next";

type TaskTopBarProps = {
  taskId?: string | null;
  activeSessionId?: string | null;
  taskTitle?: string;
  /** `owner/repo` (or the repository name) of the task's primary repository. */
  repositoryLabel?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  workflowSteps?: WorkflowStepperStep[];
  currentStepId?: string | null;
  workflowId?: string | null;
  workspaceId?: string | null;
  projectId?: string | null;
  issueUrl?: string;
  issueNumber?: number;
  isArchived?: boolean;
  embeddedVscodeSupported?: boolean;
  remoteExecutorType?: string | null;
  officeTaskHref?: string | null;
  onTaskUnarchived?: (taskId: string) => void;
  onMoveStart?: () => void;
  onMoveError?: (error: unknown) => void;
};

const TaskTopBar = memo(function TaskTopBar({
  taskId,
  activeSessionId,
  taskTitle,
  repositoryLabel,
  showDebugOverlay,
  onToggleDebugOverlay,
  workflowSteps,
  currentStepId,
  workflowId,
  workspaceId,
  projectId,
  isArchived,
  embeddedVscodeSupported,
  issueUrl,
  issueNumber,
  remoteExecutorType,
  officeTaskHref,
  onTaskUnarchived,
  onMoveStart,
  onMoveError,
}: TaskTopBarProps) {
  const { t } = useTranslation();
  // Projects only exist for office-owned tasks, so kanban-mode tasks render no
  // ancestry trail at all.
  const project = useOfficeProject(projectId);
  const showExecutorSettings =
    !isArchived && shouldShowExecutorEnvironmentControls(remoteExecutorType);
  const parents = buildTaskCrumbs(project, repositoryLabel);
  return (
    <PageTopbar
      testId="task-topbar"
      // Same fallback the rename control renders, so the crumb's accessible
      // name and the measured width never diverge from what is shown.
      title={taskTitle ?? t("task:taskDetails")}
      titleSlot={<TaskTopBarTitle taskId={taskId} taskTitle={taskTitle} isArchived={isArchived} />}
      parents={parents}
      // This bar is desktop-only (the mobile session bar owns phones), so the
      // phone-only home crumb and status trigger have no surface here.
      homeAffordance="none"
      showStatusTrigger={false}
      leftActions={
        showExecutorSettings ? (
          <ExecutorSettingsButton taskId={taskId} sessionId={activeSessionId ?? null} />
        ) : undefined
      }
      center={
        workflowSteps && workflowSteps.length > 0 ? (
          <WorkflowStepper
            steps={workflowSteps}
            currentStepId={currentStepId ?? null}
            taskId={taskId ?? null}
            workflowId={workflowId ?? null}
            isArchived={isArchived}
            onMoveStart={onMoveStart}
            onMoveError={onMoveError}
          />
        ) : undefined
      }
      // The stepper handles its own truncation (`w-full min-w-0 overflow-hidden`),
      // so the center zone may shrink instead of pushing chrome out of the bar.
      centerClassName="min-w-0 shrink"
      actions={
        <TopBarRight
          taskId={taskId}
          activeSessionId={activeSessionId}
          showDebugOverlay={showDebugOverlay}
          onToggleDebugOverlay={onToggleDebugOverlay}
          isArchived={isArchived}
          workspaceId={workspaceId}
          embeddedVscodeSupported={embeddedVscodeSupported}
          taskTitle={taskTitle}
          issueUrl={issueUrl}
          issueNumber={issueNumber}
          officeTaskHref={officeTaskHref}
          onTaskUnarchived={onTaskUnarchived}
        />
      }
    />
  );
});

/**
 * Ancestry crumbs for the open task: its office project (when it has one) and
 * then its repository, closest to the title. The repository crumb carries no
 * `href` on purpose: it orients ("this task is in kdlbs/kandev") rather than
 * navigating, since there is no repository route to land on.
 *
 * A multi-repository task names its primary repository only, which is the same
 * one the rest of the page (branch pickers, Changes) treats as primary.
 */
function buildTaskCrumbs(
  project: { id: string; name: string } | null | undefined,
  repositoryLabel: string | null | undefined,
): ParentCrumb[] | undefined {
  const crumbs: ParentCrumb[] = [];
  if (project) crumbs.push({ label: project.name, href: `/office/projects/${project.id}` });
  if (repositoryLabel) crumbs.push({ label: repositoryLabel });
  return crumbs.length > 0 ? crumbs : undefined;
}

// IssueTrackerButtons picks the right ticket status button for a task whose
// title already carries an external issue key. Jira and Linear use the same
// TEAM-NUMBER identifier shape, so both `extract` calls would match
// "ENG-123" — we resolve ambiguity by preferring whichever integration is
// currently available for the workspace, with Jira winning the tie-break since
// it shipped first. Creating new links lives in the task Link menu.
function IssueTrackerButtons({
  workspaceId,
  taskTitle,
}: {
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
  return null;
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
  const { t } = useTranslation();
  const label = showDebugOverlay ? t("task:hideDebugInfo") : t("task:showDebugInfo");
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
  taskTitle,
  issueUrl,
  issueNumber,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  isArchived?: boolean;
  workspaceId?: string | null;
  taskTitle?: string;
  issueUrl?: string;
  issueNumber?: number;
}) {
  const { t } = useTranslation();
  return (
    <TopbarCluster
      label={t("task:taskStatusAndAttention")}
      className={[
        "[&_button]:h-7",
        "[&_button]:text-xs",
        "[&_[data-testid=issue-topbar-button]]:h-7",
        "[&_[data-testid=issue-topbar-button]]:text-xs",
        "[&_[data-testid=mr-topbar-button]]:h-7",
        "[&_[data-testid=mr-topbar-button]]:text-xs",
      ].join(" ")}
    >
      <DocumentControls activeSessionId={activeSessionId ?? null} />
      {!isArchived && (
        <>
          <PortForwardButton sessionId={activeSessionId} />
          {/* PR (GitHub) and MR (GitLab) buttons each render nothing when no
              rows match, so showing both covers GitHub-only, GitLab-only, and
              multi-repo tasks without needing an explicit provider switch. */}
          <GitHubIssueTopbarButton issueUrl={issueUrl} issueNumber={issueNumber} />
          <PRTopbarButton />
          <MRTopbarButton />
          <RegisteredChangeRequestStatus
            taskId={taskId ?? null}
            sessionId={activeSessionId}
            surface="topbar"
          />
          <IssueTrackerButtons workspaceId={workspaceId} taskTitle={taskTitle} />
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
  const { t } = useTranslation();
  if (!issueUrl) return null;
  const label = issueNumber ? `#${issueNumber}` : t("task:issue");
  const tooltip = issueNumber
    ? t("task:githubIssueNumbered", { number: issueNumber })
    : t("task:githubIssue2");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          data-testid="issue-topbar-button"
          data-issue-number={issueNumber}
          size="sm"
          variant="outline"
          className="cursor-pointer gap-1.5 px-2"
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
  embeddedVscodeSupported,
}: {
  activeSessionId?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
  embeddedVscodeSupported?: boolean;
}) {
  const { t } = useTranslation();
  const showDebugToggle = isDebugUI() && onToggleDebugOverlay;

  return (
    <TopbarCluster label={t("task:taskTools")} className="[&_button]:h-7 [&_button]:text-xs">
      {!isArchived && (
        <>
          <LayoutPresetSelector />
          <EditorsMenu
            activeSessionId={activeSessionId ?? null}
            embeddedVscodeSupported={embeddedVscodeSupported ?? false}
          />
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
  embeddedVscodeSupported,
  taskTitle,
  issueUrl,
  issueNumber,
  officeTaskHref,
  onTaskUnarchived,
}: {
  taskId?: string | null;
  activeSessionId?: string | null;
  showDebugOverlay?: boolean;
  onToggleDebugOverlay?: () => void;
  isArchived?: boolean;
  workspaceId?: string | null;
  embeddedVscodeSupported?: boolean;
  taskTitle?: string;
  issueUrl?: string;
  issueNumber?: number;
  officeTaskHref?: string | null;
  onTaskUnarchived?: (taskId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-self-end gap-2 [&_button]:whitespace-nowrap">
      <TopbarMetrics activeSessionId={activeSessionId} size="sm" />
      {!isArchived && (
        <TopbarCluster
          label={t("task:pluginTopBarActions")}
          className="[&_button]:h-7 [&_button]:text-xs"
        >
          <TaskTopBarPluginActions
            sessionId={activeSessionId ?? null}
            taskId={taskId ?? null}
            taskTitle={taskTitle}
            workspaceId={workspaceId ?? null}
          />
        </TopbarCluster>
      )}
      {isArchived && (
        <TopbarCluster
          label={t("task:unarchiveTask")}
          className="[&_button]:h-7 [&_button]:text-xs"
        >
          <TaskUnarchiveButton taskId={taskId} onUnarchived={onTaskUnarchived} />
        </TopbarCluster>
      )}
      {officeTaskHref && (
        <TopbarCluster label={t("task:openInOfficeView")} className="[&_a]:h-7 [&_a]:text-xs">
          <Button asChild size="sm" variant="outline" className="h-7 cursor-pointer px-2">
            <Link href={officeTaskHref}>{t("task:openInOfficeView")}</Link>
          </Button>
        </TopbarCluster>
      )}
      <AttentionStatusGroup
        taskId={taskId}
        activeSessionId={activeSessionId}
        isArchived={isArchived}
        workspaceId={workspaceId}
        taskTitle={taskTitle}
        issueUrl={issueUrl}
        issueNumber={issueNumber}
      />
      <TopbarToolsGroup
        activeSessionId={activeSessionId}
        showDebugOverlay={showDebugOverlay}
        onToggleDebugOverlay={onToggleDebugOverlay}
        isArchived={isArchived}
        embeddedVscodeSupported={embeddedVscodeSupported}
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
    case "k8s":
      return true;
    default:
      return false;
  }
}

export { TaskTopBar };
