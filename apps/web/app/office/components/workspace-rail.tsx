"use client";

import { useRouter } from "next/navigation";
import { IconPlus, IconArrowLeft } from "@tabler/icons-react";
import Link from "next/link";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";

const GRADIENTS = [
  "linear-gradient(135deg, #6366f1, #8b5cf6)",
  "linear-gradient(135deg, #3b82f6, #06b6d4)",
  "linear-gradient(135deg, #10b981, #06b6d4)",
  "linear-gradient(135deg, #f59e0b, #ef4444)",
  "linear-gradient(135deg, #ec4899, #8b5cf6)",
  "linear-gradient(135deg, #14b8a6, #3b82f6)",
  "linear-gradient(135deg, #f97316, #facc15)",
  "linear-gradient(135deg, #84cc16, #10b981)",
];

function getWorkspaceGradient(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  }
  return GRADIENTS[hash % GRADIENTS.length];
}

function getInitials(name: string): string {
  const words = (name || "W").trim().split(/\s+/);
  if (words.length >= 2) {
    return (words[0].charAt(0) + words[1].charAt(0)).toUpperCase();
  }
  return words[0].charAt(0).toUpperCase();
}

interface WorkspaceItem {
  id: string;
  name: string;
}

interface WorkspaceRailProps {
  workspaces: WorkspaceItem[];
  activeWorkspaceId: string | null;
}

export function WorkspaceRail({
  workspaces: ssrWorkspaces,
  activeWorkspaceId: ssrActiveId,
}: WorkspaceRailProps) {
  const router = useRouter();
  const storeWorkspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);

  // Use store if hydrated, fall back to SSR props
  const items = storeWorkspaces.items.length > 0 ? storeWorkspaces.items : ssrWorkspaces;
  const activeId = storeWorkspaces.activeId ?? ssrActiveId;

  const handleSelect = (id: string) => {
    setActiveWorkspace(id);
    router.push(`/office?workspaceId=${id}`);
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
      <div className="flex flex-col items-center gap-2 overflow-y-auto flex-1 w-full pt-1">
        {items.map((ws) => {
          const isActive = ws.id === activeId;
          const initials = getInitials(ws.name);
          const gradient = getWorkspaceGradient(ws.id);
          return (
            <Tooltip key={ws.id}>
              <TooltipTrigger asChild>
                <div className="group relative flex items-center justify-center w-full">
                  {/* Left edge indicator pill */}
                  <div
                    className={`absolute left-0 top-1/2 -translate-y-1/2 w-[3px] rounded-r bg-foreground transition-all duration-300 ${
                      isActive ? "h-3 opacity-100" : "h-0 opacity-0"
                    }`}
                  />
                  <button
                    onClick={() => handleSelect(ws.id)}
                    style={{ background: gradient }}
                    className={`h-11 w-11 rounded-[14px] flex items-center justify-center text-sm font-black tracking-tight text-white cursor-pointer transition-all duration-200 select-none ${
                      isActive
                        ? "ring-2 ring-white/30 ring-offset-2 ring-offset-background scale-95"
                        : "opacity-60 hover:opacity-100 hover:scale-105 hover:rounded-[10px]"
                    }`}
                  >
                    {initials}
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
              aria-label="Add workspace"
              onClick={() => router.push("/office/setup?mode=new")}
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
