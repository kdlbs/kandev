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
    <div className="w-[60px] h-full border-r border-border bg-background flex flex-col items-center py-3 gap-2 shrink-0">
      {/* Back to homepage */}
      <Tooltip>
        <TooltipTrigger asChild>
          <Link
            href="/"
            className="h-8 w-8 flex items-center justify-center rounded-md hover:bg-accent transition-colors cursor-pointer mb-2"
          >
            <IconArrowLeft className="h-4 w-4 text-muted-foreground" />
          </Link>
        </TooltipTrigger>
        <TooltipContent side="right">Back to board</TooltipContent>
      </Tooltip>

      {/* Workspace avatars */}
      <div className="flex-1 flex flex-col gap-2 overflow-y-auto">
        {workspaces.items.map((ws) => {
          const isActive = ws.id === workspaces.activeId;
          const initial = (ws.name || "W").charAt(0).toUpperCase();
          return (
            <Tooltip key={ws.id}>
              <TooltipTrigger asChild>
                <div className="relative flex items-center">
                  {/* Active indicator bar */}
                  <div
                    className={`absolute left-0 w-1 rounded-r-full transition-all ${
                      isActive ? "h-6 bg-primary" : "h-0"
                    }`}
                  />
                  <button
                    onClick={() => handleSelect(ws.id)}
                    className={`h-10 w-10 rounded-lg flex items-center justify-center text-sm font-semibold cursor-pointer transition-colors ml-1 ${
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted text-muted-foreground hover:bg-accent"
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
      </div>

      {/* Add workspace button */}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-10 w-10 rounded-full border border-dashed border-border cursor-pointer"
          >
            <IconPlus className="h-4 w-4 text-muted-foreground" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="right">Add workspace</TooltipContent>
      </Tooltip>
    </div>
  );
}
