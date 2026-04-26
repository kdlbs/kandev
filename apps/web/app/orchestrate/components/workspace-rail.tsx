"use client";

import { useRouter } from "next/navigation";
import { IconPlus, IconArrowLeft } from "@tabler/icons-react";
import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";

export function WorkspaceRail() {
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);

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

      {/* Workspace avatars + add button together */}
      <div className="flex flex-col items-center gap-3 overflow-y-auto flex-1">
        {workspaces.items.map((ws) => {
          const isActive = ws.id === workspaces.activeId;
          const initial = (ws.name || "W").charAt(0).toUpperCase();
          return (
            <Tooltip key={ws.id}>
              <TooltipTrigger asChild>
                <button
                  onClick={() => handleSelect(ws.id)}
                  className={`relative h-11 w-11 rounded-xl flex items-center justify-center text-sm font-semibold cursor-pointer transition-all ${
                    isActive
                      ? "bg-primary text-primary-foreground shadow-[0_0_0_2px] shadow-primary/30"
                      : "bg-muted text-muted-foreground hover:bg-accent"
                  }`}
                >
                  {initial}
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">{ws.name}</TooltipContent>
            </Tooltip>
          );
        })}

        {/* Add workspace button - right below workspaces */}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-11 w-11 rounded-full border-2 border-dashed border-muted-foreground/30 cursor-pointer hover:border-muted-foreground/50"
            >
              <IconPlus className="h-4 w-4 text-muted-foreground/50" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Add workspace</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
