"use client";

import { useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { IconCheck, IconChevronDown, IconFolder, IconPlus } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { cn } from "@/lib/utils";
import { getWorkspaceGradient, getWorkspaceInitials } from "./workspace-gradient";

type AppSidebarWorkspacePickerProps = {
  collapsed: boolean;
};

type TriggerProps = {
  collapsed: boolean;
  activeId: string | null;
  activeName: string;
};

function OfficeTrigger({ collapsed, activeId, activeName }: TriggerProps) {
  return (
    <button
      type="button"
      className={cn(
        "flex items-center gap-2 rounded-md hover:bg-muted/60 cursor-pointer",
        collapsed ? "p-0.5" : "flex-1 min-w-0 px-1.5 py-1",
      )}
      aria-label="Switch workspace"
    >
      <span
        className="h-7 w-7 rounded-md flex items-center justify-center text-[11px] font-black text-white shrink-0 select-none"
        style={{
          background: activeId ? getWorkspaceGradient(activeId) : "var(--muted)",
        }}
      >
        {getWorkspaceInitials(activeName)}
      </span>
      {!collapsed && (
        <span className="flex-1 min-w-0 text-sm font-semibold truncate text-left sidebar-fade-in">
          {activeName}
        </span>
      )}
    </button>
  );
}

function MinimalTrigger({ collapsed, activeName }: TriggerProps) {
  return (
    <button
      type="button"
      className={cn(
        "flex items-center gap-1.5 rounded-md hover:bg-muted/60 cursor-pointer text-foreground/80",
        collapsed
          ? "h-7 w-7 justify-center mx-auto p-0"
          : "flex-1 min-w-0 px-1.5 py-1 text-sm font-semibold",
      )}
      aria-label="Switch workspace"
    >
      {collapsed ? (
        <IconFolder className="h-4 w-4" />
      ) : (
        <>
          <span className="flex-1 min-w-0 truncate text-left sidebar-fade-in">{activeName}</span>
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/70 shrink-0" />
        </>
      )}
    </button>
  );
}

export function AppSidebarWorkspacePicker({ collapsed }: AppSidebarWorkspacePickerProps) {
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const workspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const [open, setOpen] = useState(false);

  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const activeId = activeWorkspace?.id ?? null;
  const activeName = activeWorkspace?.name ?? "Workspace";

  const handleSelect = useCallback(
    (id: string) => {
      document.cookie = `office-active-workspace=${id}; path=/; max-age=86400; samesite=strict; secure`;
      setActiveWorkspace(id);
      if (officeEnabled) {
        router.push(`/office?workspaceId=${id}`);
      }
      setOpen(false);
    },
    [router, setActiveWorkspace, officeEnabled],
  );

  const triggerProps: TriggerProps = { collapsed, activeId, activeName };

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        {officeEnabled ? <OfficeTrigger {...triggerProps} /> : <MinimalTrigger {...triggerProps} />}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-60">
        {workspaces.items.length === 0 ? (
          <DropdownMenuItem disabled>No workspaces</DropdownMenuItem>
        ) : (
          workspaces.items.map((ws) => (
            <DropdownMenuItem
              key={ws.id}
              onClick={() => handleSelect(ws.id)}
              className="cursor-pointer gap-2"
            >
              {officeEnabled && (
                <span
                  className="h-5 w-5 rounded-sm flex items-center justify-center text-[9px] font-black text-white shrink-0"
                  style={{ background: getWorkspaceGradient(ws.id) }}
                >
                  {getWorkspaceInitials(ws.name)}
                </span>
              )}
              <span className="flex-1 truncate">{ws.name}</span>
              {ws.id === activeId && <IconCheck className="h-3.5 w-3.5" />}
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          onClick={() => {
            router.push("/office/setup?mode=new");
            setOpen(false);
          }}
        >
          <IconPlus className="h-3.5 w-3.5" />
          <span>Add workspace</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
