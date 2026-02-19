"use client";

import { IconCheck, IconChevronDown } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type Workspace = {
  id: string;
  name: string;
};

type WorkspaceSwitcherProps = {
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  onSelect: (workspaceId: string) => void;
};

export function WorkspaceSwitcher({
  workspaces,
  activeWorkspaceId,
  onSelect,
}: WorkspaceSwitcherProps) {
  const selectedWorkspace = workspaces.find((w) => w.id === activeWorkspaceId);

  // If only one workspace, show just the name without dropdown
  if (workspaces.length <= 1) {
    return (
      <span className="text-sm font-medium text-muted-foreground truncate">
        {selectedWorkspace?.name || "Workspace"}
      </span>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          title="Switch Workspace"
          className={cn(
            "group flex items-center gap-1 text-sm font-medium cursor-pointer",
            "text-muted-foreground hover:text-foreground transition-colors duration-150",
            "focus:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded",
          )}
        >
          <span className="truncate">{selectedWorkspace?.name || "Workspace"}</span>
          <IconChevronDown
            className={cn(
              "h-3.5 w-3.5 shrink-0 opacity-0 group-hover:opacity-50 transition-opacity duration-150",
            )}
          />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        {workspaces.map((workspace) => (
          <DropdownMenuItem
            key={workspace.id}
            onClick={() => onSelect(workspace.id)}
            className={cn(
              "justify-between",
              activeWorkspaceId === workspace.id && "bg-foreground/10",
            )}
          >
            <span className="truncate">{workspace.name}</span>
            {activeWorkspaceId === workspace.id && <IconCheck className="h-4 w-4 shrink-0" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
