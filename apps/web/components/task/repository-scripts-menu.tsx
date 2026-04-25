"use client";

import { IconDeviceDesktop, IconPlayerPlay } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";

/**
 * Returns the trimmed dev_script command of the active session's repository,
 * or an empty string when none is configured. Boolean usage stays valid via
 * truthiness; callers that need the command itself can use it directly.
 */
export function useActiveSessionDevScript(): string {
  return useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return "";
    const repoId = state.taskSessions.items[sessionId]?.repository_id;
    if (!repoId) return "";
    const allRepos = Object.values(state.repositories.itemsByWorkspaceId).flat();
    const repo = allRepos.find((r) => r.id === repoId);
    return repo?.dev_script?.trim() ?? "";
  });
}

/**
 * Renders custom repository scripts (and the dev script if configured) as
 * dropdown items inside the dockview "+" menu. Returns null when neither is
 * available so the caller doesn't render an empty section.
 */
export function RepositoryScriptsMenuItems({
  onRunScript,
  onRunDevScript,
}: {
  onRunScript: (scriptId: string) => void;
  onRunDevScript: () => void;
}) {
  const repositoryId = useAppStore((s) => {
    const sessionId = s.tasks.activeSessionId;
    if (!sessionId) return null;
    return s.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const { scripts } = useRepositoryScripts(repositoryId);
  const devScript = useActiveSessionDevScript();

  if (scripts.length === 0 && !devScript) return null;

  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuLabel className="text-xs text-muted-foreground">Scripts</DropdownMenuLabel>
      {devScript && (
        <DropdownMenuItem
          onClick={onRunDevScript}
          className="cursor-pointer text-xs"
          data-testid="run-dev-script"
        >
          <IconDeviceDesktop className="h-3.5 w-3.5 mr-1.5 shrink-0" />
          <span className="truncate">Dev Server</span>
        </DropdownMenuItem>
      )}
      {scripts.map((script) => (
        <DropdownMenuItem
          key={script.id}
          onClick={() => onRunScript(script.id)}
          className="cursor-pointer text-xs"
          data-testid={`run-script-${script.id}`}
        >
          <IconPlayerPlay className="h-3.5 w-3.5 mr-1.5 shrink-0" />
          <span className="truncate">{script.name}</span>
        </DropdownMenuItem>
      ))}
    </>
  );
}
