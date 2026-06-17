"use client";

import { IconArrowsShuffle, IconFolder, IconGitBranch } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/workspace";

type WorkspacesGroupProps = {
  pathname: string;
  expanded?: boolean;
  onToggle?: () => void;
};

function isWorkspaceRoute(pathname: string, workspaceId: string): boolean {
  const workspacePath = `${ROOT_HREF}/${workspaceId}`;
  return pathname === workspacePath || pathname.startsWith(`${workspacePath}/`);
}

export function WorkspacesGroup({ pathname, expanded, onToggle }: WorkspacesGroupProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeWorkspaceId =
    workspaces.find((workspace) => isWorkspaceRoute(pathname, workspace.id))?.id ?? null;
  const routeExpansionKey = activeWorkspaceId ?? "all";
  const hasActiveWorkspaceRoute = activeWorkspaceId !== null;

  function shouldExpandWorkspace(workspaceId: string): boolean {
    return !hasActiveWorkspaceRoute || activeWorkspaceId === workspaceId;
  }

  return (
    <SettingsGroup
      label="Workspaces"
      icon={IconFolder}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      expanded={expanded}
      onToggle={onToggle}
    >
      {workspaces.map((workspace) => {
        const workspacePath = `${ROOT_HREF}/${workspace.id}`;
        const repositoriesPath = `${workspacePath}/repositories`;
        const workflowsPath = `${workspacePath}/workflows`;
        return (
          <SettingsGroup
            key={`${workspace.id}:${routeExpansionKey}`}
            label={workspace.name}
            href={workspacePath}
            isActive={pathname === workspacePath}
            defaultExpanded={shouldExpandWorkspace(workspace.id)}
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
          </SettingsGroup>
        );
      })}
    </SettingsGroup>
  );
}
