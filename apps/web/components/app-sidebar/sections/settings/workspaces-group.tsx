"use client";

import { useEffect, useState, type ComponentType } from "react";
import {
  IconArrowsShuffle,
  IconBolt,
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandSentry,
  IconBrandSlack,
  IconFolder,
  IconGitBranch,
  IconHexagon,
  IconPlugConnected,
  IconTicket,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import { useAzureDevOpsAvailable } from "@/hooks/domains/azure-devops/use-azure-devops-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import { useJiraAuthed } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAuthed } from "@/hooks/domains/linear/use-linear-availability";
import { useSentryAvailable } from "@/hooks/domains/sentry/use-sentry-availability";
import { useSlackAuthed } from "@/hooks/domains/slack/use-slack-availability";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/workspace";

type IntegrationIcon = ComponentType<{ className?: string }>;

const INTEGRATIONS: Array<{ slug: string; label: string; icon: IntegrationIcon }> = [
  { slug: "azure-devops", label: "Azure DevOps", icon: AzureDevOpsIcon },
  { slug: "github", label: "GitHub", icon: IconBrandGithub },
  { slug: "gitlab", label: "GitLab", icon: IconBrandGitlab },
  { slug: "jira", label: "Jira", icon: IconTicket },
  { slug: "linear", label: "Linear", icon: IconHexagon },
  { slug: "sentry", label: "Sentry", icon: IconBrandSentry },
  { slug: "slack", label: "Slack", icon: IconBrandSlack },
];

const ACTIVE_WORKSPACE_LABEL = (
  <span className="shrink-0 rounded-full border border-primary/35 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
    Active
  </span>
);

const ENABLED_LABEL = (
  <span className="shrink-0 rounded border border-emerald-500/30 bg-emerald-500/10 px-1 py-0.5 text-[9px] font-medium leading-none text-emerald-600 dark:text-emerald-400">
    Enabled
  </span>
);

function WorkspaceIntegrationItems({
  workspaceId,
  integrationsPath,
  pathname,
}: {
  workspaceId: string;
  integrationsPath: string;
  pathname: string;
}) {
  const azureDevOps = useAzureDevOpsAvailable(workspaceId);
  const { status: githubStatus } = useGitHubStatus(workspaceId);
  const gitlab = useGitLabAvailable();
  const jira = useJiraAuthed(workspaceId);
  const linear = useLinearAuthed(workspaceId);
  const sentry = useSentryAvailable(workspaceId);
  const slack = useSlackAuthed(workspaceId);
  const enabled = new Set([
    ...(azureDevOps ? ["azure-devops"] : []),
    ...(githubStatus?.authenticated || githubStatus?.token_configured ? ["github"] : []),
    ...(gitlab ? ["gitlab"] : []),
    ...(jira ? ["jira"] : []),
    ...(linear ? ["linear"] : []),
    ...(sentry ? ["sentry"] : []),
    ...(slack ? ["slack"] : []),
  ]);

  return INTEGRATIONS.map(({ slug, label, icon }) => {
    const href = `${integrationsPath}/${slug}`;
    return (
      <SettingsLeaf
        key={href}
        href={href}
        label={label}
        labelSuffix={enabled.has(slug) ? ENABLED_LABEL : undefined}
        icon={icon}
        isActive={pathname === href}
        depth={3}
      />
    );
  });
}

type WorkspacesGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

function isWorkspaceRoute(pathname: string, workspaceId: string): boolean {
  const workspacePath = `${ROOT_HREF}/${workspaceId}`;
  return pathname === workspacePath || pathname.startsWith(`${workspacePath}/`);
}

function activeWorkspaceIdFor(
  workspaces: Array<{ id: string }>,
  storeActiveWorkspaceId: string | null,
): string | null {
  return workspaces.some((workspace) => workspace.id === storeActiveWorkspaceId)
    ? storeActiveWorkspaceId
    : null;
}

function activeWorkspaceFirst<T extends { id: string }>(workspaces: T[], activeId: string | null) {
  if (!activeId) return workspaces;
  return [
    ...workspaces.filter((workspace) => workspace.id === activeId),
    ...workspaces.filter((workspace) => workspace.id !== activeId),
  ];
}

export function WorkspacesGroup({ pathname, expanded, onToggle }: WorkspacesGroupProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const storeActiveWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const routeWorkspaceId =
    workspaces.find((workspace) => isWorkspaceRoute(pathname, workspace.id))?.id ?? null;
  const activeWorkspaceId = activeWorkspaceIdFor(workspaces, storeActiveWorkspaceId);
  const orderedWorkspaces = activeWorkspaceFirst(workspaces, activeWorkspaceId);
  const defaultExpandedWorkspaceId = routeWorkspaceId ?? activeWorkspaceId ?? workspaces[0]?.id;
  const [expandedWorkspaceId, setExpandedWorkspaceId] = useState<string | null>(
    defaultExpandedWorkspaceId ?? null,
  );

  useEffect(() => {
    setExpandedWorkspaceId(defaultExpandedWorkspaceId ?? null);
  }, [defaultExpandedWorkspaceId]);

  const toggleWorkspace = (workspaceId: string) => {
    setExpandedWorkspaceId((current) => (current === workspaceId ? null : workspaceId));
  };

  return (
    <SettingsGroup
      label="Workspaces"
      icon={IconFolder}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {orderedWorkspaces.map((workspace) => {
        const workspacePath = `${ROOT_HREF}/${workspace.id}`;
        const repositoriesPath = `${workspacePath}/repositories`;
        const workflowsPath = `${workspacePath}/workflows`;
        const automationsPath = `${workspacePath}/automations`;
        const integrationsPath = `${workspacePath}/integrations`;
        const integrationsActive =
          pathname === integrationsPath || pathname.startsWith(`${integrationsPath}/`);
        const workspaceIsActive = activeWorkspaceId === workspace.id;
        return (
          <SettingsGroup
            key={workspace.id}
            label={workspace.name}
            labelSuffix={workspaceIsActive ? ACTIVE_WORKSPACE_LABEL : undefined}
            href={workspacePath}
            isActive={pathname === workspacePath}
            expanded={expandedWorkspaceId === workspace.id}
            onToggle={() => toggleWorkspace(workspace.id)}
            depth={1}
          >
            <SettingsLeaf
              href={repositoriesPath}
              label="Repositories"
              icon={IconGitBranch}
              isActive={pathname === repositoriesPath}
              depth={2}
            />
            <SettingsLeaf
              href={workflowsPath}
              label="Workflows"
              icon={IconArrowsShuffle}
              isActive={pathname === workflowsPath}
              depth={2}
            />
            <SettingsGroup
              label="Integrations"
              icon={IconPlugConnected}
              href={integrationsPath}
              isActive={pathname === integrationsPath}
              defaultExpanded={integrationsActive}
              depth={2}
            >
              <WorkspaceIntegrationItems
                workspaceId={workspace.id}
                integrationsPath={integrationsPath}
                pathname={pathname}
              />
            </SettingsGroup>
            <SettingsLeaf
              href={automationsPath}
              label="Automations"
              icon={IconBolt}
              isActive={pathname === automationsPath}
              depth={2}
            />
          </SettingsGroup>
        );
      })}
    </SettingsGroup>
  );
}
