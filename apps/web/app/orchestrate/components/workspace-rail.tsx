"use client";

import { useRouter } from "next/navigation";
import { IconPlus, IconArrowLeft } from "@tabler/icons-react";
import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";

interface WorkspaceItem {
  id: string;
  name: string;
}

interface WorkspaceRailProps {
  workspaces: WorkspaceItem[];
  activeWorkspaceId: string | null;
}

export function WorkspaceRail({ workspaces: ssrWorkspaces, activeWorkspaceId: ssrActiveId }: WorkspaceRailProps) {
  const router = useRouter();
  const storeWorkspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);

  // Use store if hydrated, fall back to SSR props
  const items = storeWorkspaces.items.length > 0 ? storeWorkspaces.items : ssrWorkspaces;
  const activeId = storeWorkspaces.activeId ?? ssrActiveId;

  const handleSelect = (id: string) => {
    setActiveWorkspace(id);
    router.push(`/orchestrate?workspaceId=${id}`);
  };

  return (
    <div className="w-[60px] h-full border-r border-border bg-background flex flex-col items-center py-3 shrink-0">
      {/* Back to homepage */}
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            href="/"
            className="h-8 w-8 flex items-center justify-center rounded-md hover:bg-accent transition-colors cursor-pointer mb-3"
          >
            <IconArrowLeft className="h-4 w-4 text-muted-foreground" />
          </Link>
        </TooltipTrigger>
        <TooltipContent side="right">Back to board</TooltipContent>
      </Tooltip>

      {/* Workspace avatars + add button */}
      <div className="flex flex-col items-center gap-2 overflow-y-auto flex-1 w-full">
        {items.map((ws) => {
          const isActive = ws.id === activeId;
          const initial = (ws.name || "W").charAt(0).toUpperCase();
          return (
            <Tooltip key={ws.id}>
              <TooltipTrigger asChild>
                <div className="group relative flex items-center justify-center w-full">
                  {/* Left edge indicator pill */}
                  {isActive && (
                    <div className="absolute left-0 top-1/2 -translate-y-1/2 w-[2px] rounded-r-full bg-foreground transition-all h-4" />
                  )}
                  <button
                    onClick={() => handleSelect(ws.id)}
                    className={`h-12 w-12 rounded-2xl flex items-center justify-center text-sm font-semibold cursor-pointer transition-all duration-200 ${
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted text-muted-foreground hover:bg-accent hover:rounded-xl"
                    }`}
                  >
                    {initial}
                  </button>
                </div>
              </TooltipTrigger>
              <TooltipContent side="right">{ws.name}</TooltipContent>
            </Tooltip>
          );
        })}

        {/* Add workspace button */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-12 w-12 rounded-full border-2 border-dashed border-muted-foreground/25 cursor-pointer hover:border-muted-foreground/40 hover:bg-accent/50"
            >
              <IconPlus className="h-4 w-4 text-muted-foreground/40" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Add workspace</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
