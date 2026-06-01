"use client";

import { IconDeviceDesktop, IconPlayerPlay } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { useAllRepositories } from "@/hooks/domains/workspace/use-all-repositories";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";

/**
 * Returns the trimmed dev_script command of the active session's repository,
 * or an empty string when none is configured. Boolean usage stays valid via
 * truthiness; callers that need the command itself can use it directly.
 */
export function useActiveSessionDevScript(): string {
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repoId = useTaskSessionById(sessionId)?.repository_id ?? null;
  // Observe cached repo lists (no fetch) to find the dev_script for repoId.
  const { repositories } = useAllRepositories(false);
  if (!repoId) return "";
  const repo = repositories.find((r) => r.id === repoId);
  return repo?.dev_script?.trim() ?? "";
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
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const repositoryId = useTaskSessionById(sessionId)?.repository_id ?? null;
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
