"use client";

import { forwardRef, useCallback, useState, type ComponentPropsWithoutRef } from "react";
import { useRouter } from "@/lib/routing/client-router";
import {
  IconBriefcase,
  IconCheck,
  IconChevronDown,
  IconLayoutKanban,
  IconPlus,
} from "@tabler/icons-react";
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
import {
  rememberLastOfficeWorkspace,
  rememberLastKanbanWorkspace,
} from "./app-sidebar-workspace-navigation";

/**
 * Compact, secondary workspace switcher inlined after the Kandev brand in the
 * sidebar header. Muted by default so the brand stays primary; the active
 * workspace name truncates with a small chevron hinting the dropdown. Only
 * rendered while the sidebar is expanded — the collapsed rail omits it.
 *
 * forwardRef + prop spread so `DropdownMenuTrigger asChild` can wire the trigger
 * (ref, onClick, aria-*, data-state) onto the underlying button.
 */
const WorkspaceTrigger = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<"button"> & { activeName: string }
>(function WorkspaceTrigger({ activeName, className, ...props }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      data-testid="sidebar-workspace-trigger"
      aria-label="Switch workspace"
      className={cn(
        "group/ws flex min-w-0 flex-1 items-center gap-1 rounded-md px-1.5 py-1 text-sm font-medium text-muted-foreground hover:bg-muted/60 hover:text-foreground cursor-pointer transition-colors",
        className,
      )}
      {...props}
    >
      <span className="min-w-0 flex-1 truncate text-left sidebar-fade-in">{activeName}</span>
      <IconChevronDown className="h-3.5 w-3.5 shrink-0 opacity-50 transition-opacity group-hover/ws:opacity-80" />
    </button>
  );
});

type WorkspaceType = "kanban" | "office";

type WorkspaceItem = {
  id: string;
  name: string;
  office_workflow_id?: string | null;
};

function workspaceType(workspace: WorkspaceItem | undefined): WorkspaceType {
  return workspace?.office_workflow_id ? "office" : "kanban";
}

function workspaceTypeLabel(type: WorkspaceType) {
  return type === "office" ? "Office" : "Kanban";
}

function WorkspaceTypeIcon({ type, className }: { type: WorkspaceType; className: string }) {
  if (type === "office") {
    return <IconBriefcase className={className} />;
  }
  return <IconLayoutKanban className={className} />;
}

export function AppSidebarWorkspacePicker() {
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const workspaces = useAppStore((s) => s.workspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const [open, setOpen] = useState(false);

  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const activeId = activeWorkspace?.id ?? null;
  const activeName = activeWorkspace?.name ?? "Workspace";

  const handleSelect = useCallback(
    (workspace: WorkspaceItem) => {
      const { id } = workspace;
      if (id === activeId) {
        setOpen(false);
        return;
      }
      const type = workspaceType(workspace);
      if (type === "office") {
        rememberLastOfficeWorkspace(workspace);
      }
      rememberLastKanbanWorkspace(workspace);
      setActiveWorkspace(id);
      if (officeEnabled) {
        const target = type === "office" ? "/office" : "/";
        router.push(`${target}?workspaceId=${id}`);
      }
      setOpen(false);
    },
    [activeId, router, setActiveWorkspace, officeEnabled],
  );

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <WorkspaceTrigger activeName={activeName} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        {workspaces.items.length === 0 ? (
          <DropdownMenuItem disabled>No workspaces</DropdownMenuItem>
        ) : (
          workspaces.items.map((ws) => {
            const type = workspaceType(ws);
            return (
              <DropdownMenuItem
                key={ws.id}
                data-testid={`sidebar-workspace-item-${ws.id}`}
                onSelect={() => handleSelect(ws)}
                className="cursor-pointer gap-2"
              >
                <WorkspaceTypeIcon
                  type={type}
                  className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
                />
                <span className="min-w-0 flex-1 truncate">{ws.name}</span>
                <span className="shrink-0 rounded border border-border/60 px-1.5 py-0.5 text-[10px] font-medium leading-none text-muted-foreground">
                  {workspaceTypeLabel(type)}
                </span>
                {ws.id === activeId && <IconCheck className="h-3.5 w-3.5 shrink-0" />}
              </DropdownMenuItem>
            );
          })
        )}
        <DropdownMenuSeparator />
        {officeEnabled ? (
          <>
            <DropdownMenuItem
              className="cursor-pointer gap-2"
              onSelect={() => router.push("/settings/workspace")}
            >
              <IconLayoutKanban className="h-3.5 w-3.5" />
              <span>New kanban workspace</span>
            </DropdownMenuItem>
            <DropdownMenuItem
              className="cursor-pointer gap-2"
              onSelect={() => router.push("/office/setup?mode=new")}
            >
              <IconBriefcase className="h-3.5 w-3.5" />
              <span>New office workspace</span>
            </DropdownMenuItem>
          </>
        ) : (
          <DropdownMenuItem
            className="cursor-pointer gap-2"
            onSelect={() => router.push("/settings/workspace")}
          >
            <IconPlus className="h-3.5 w-3.5" />
            <span>Add workspace</span>
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
