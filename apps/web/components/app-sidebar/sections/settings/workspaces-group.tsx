"use client";

import { IconArrowsShuffle, IconFolder, IconGitBranch } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { SettingsGroup, SettingsLeaf } from "./settings-nav-primitives";

const ROOT_HREF = "/settings/workspace";

type WorkspacesGroupProps = {
  pathname: string;
};

export function WorkspacesGroup({ pathname }: WorkspacesGroupProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const isWorkspace = pathname.startsWith(ROOT_HREF);

  return (
    <SettingsGroup
      label="Workspaces"
      icon={IconFolder}
      href={ROOT_HREF}
      isActive={pathname === ROOT_HREF}
      defaultExpanded={isWorkspace}
    >
      {workspaces.map((workspace) => {
        const workspacePath = `${ROOT_HREF}/${workspace.id}`;
        const repositoriesPath = `${workspacePath}/repositories`;
        const workflowsPath = `${workspacePath}/workflows`;
        return (
          <SettingsGroup
            key={workspace.id}
            label={workspace.name}
            href={workspacePath}
            isActive={pathname === workspacePath}
            defaultExpanded={pathname.startsWith(workspacePath)}
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
