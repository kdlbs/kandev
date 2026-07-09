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
  workspaceHomeHref,
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
  ComponentPropsWithoutRef<"button"> & { activeName: string; chevronTestId: string }
>(function WorkspaceTrigger({ activeName, chevronTestId, className, ...props }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      data-testid="sidebar-workspace-trigger"
      aria-label="Switch workspace"
      className={cn(
        "group/ws flex h-8 min-w-0 flex-1 items-center gap-1.5 rounded-md border border-border/70 bg-background px-2.5 text-sm font-medium text-foreground shadow-sm cursor-pointer transition-colors hover:border-border hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        className,
      )}
      {...props}
    >
      <span className="min-w-0 flex-1 truncate text-left sidebar-fade-in">{activeName}</span>
      <IconChevronDown
        data-testid={chevronTestId}
        className="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-colors group-hover/ws:text-foreground/80"
      />
    </button>
  );
});

type WorkspaceType = "kanban" | "office";

type WorkspaceItem = {
  id: string;
  name: string;
  office_workflow_id?: string | null;
};

type WorkspacePickerProps = {
  triggerClassName?: string;
  contentClassName?: string;
  contentAlign?: "start" | "center" | "end";
  triggerTestId?: string;
  chevronTestId?: string;
  itemTestIdPrefix?: string;
  modal?: boolean;
  onActionComplete?: () => void;
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

function rememberSelectedWorkspace(workspace: WorkspaceItem) {
  if (workspaceType(workspace) === "office") {
    rememberLastOfficeWorkspace(workspace);
  } else {
    rememberLastKanbanWorkspace(workspace);
  }
}

type WorkspacePickerContentProps = {
  workspaces: WorkspaceItem[];
  activeId: string | null;
  itemTestIdPrefix: string;
  officeEnabled: boolean;
  onWorkspaceSelect: (workspace: WorkspaceItem) => void;
  onNavigate: (href: string) => void;
};

function WorkspacePickerContent({
  workspaces,
  activeId,
  itemTestIdPrefix,
  officeEnabled,
  onWorkspaceSelect,
  onNavigate,
}: WorkspacePickerContentProps) {
  return (
    <>
      <WorkspaceList
        workspaces={workspaces}
        activeId={activeId}
        itemTestIdPrefix={itemTestIdPrefix}
        onWorkspaceSelect={onWorkspaceSelect}
      />
      <DropdownMenuSeparator />
      <WorkspaceCreateItems officeEnabled={officeEnabled} onNavigate={onNavigate} />
    </>
  );
}

function WorkspaceList({
  workspaces,
  activeId,
  itemTestIdPrefix,
  onWorkspaceSelect,
}: Pick<
  WorkspacePickerContentProps,
  "workspaces" | "activeId" | "itemTestIdPrefix" | "onWorkspaceSelect"
>) {
  if (workspaces.length === 0) {
    return <DropdownMenuItem disabled>No workspaces</DropdownMenuItem>;
  }

  return workspaces.map((ws) => {
    const type = workspaceType(ws);
    return (
      <DropdownMenuItem
        key={ws.id}
        data-testid={`${itemTestIdPrefix}-${ws.id}`}
        onSelect={() => onWorkspaceSelect(ws)}
        className="cursor-pointer gap-2"
      >
        <WorkspaceTypeIcon type={type} className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate">{ws.name}</span>
        <span className="shrink-0 rounded border border-border/60 px-1.5 py-0.5 text-[10px] font-medium leading-none text-muted-foreground">
          {workspaceTypeLabel(type)}
        </span>
        {ws.id === activeId && <IconCheck className="h-3.5 w-3.5 shrink-0" />}
      </DropdownMenuItem>
    );
  });
}

function WorkspaceCreateItems({
  officeEnabled,
  onNavigate,
}: Pick<WorkspacePickerContentProps, "officeEnabled" | "onNavigate">) {
  if (!officeEnabled) {
    return (
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/settings/workspace")}
      >
        <IconPlus className="h-3.5 w-3.5" />
        <span>Add workspace</span>
      </DropdownMenuItem>
    );
  }

  return (
    <>
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/settings/workspace")}
      >
        <IconLayoutKanban className="h-3.5 w-3.5" />
        <span>New kanban workspace</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/office/setup?mode=new")}
      >
        <IconBriefcase className="h-3.5 w-3.5" />
        <span>New office workspace</span>
      </DropdownMenuItem>
    </>
  );
}

export function AppSidebarWorkspacePicker({
  triggerClassName,
  contentClassName,
  contentAlign = "start",
  triggerTestId = "sidebar-workspace-trigger",
  chevronTestId = "sidebar-workspace-trigger-chevron",
  itemTestIdPrefix = "sidebar-workspace-item",
  modal = true,
  onActionComplete,
}: WorkspacePickerProps = {}) {
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
        if (officeEnabled && workspaceType(workspace) === "kanban") {
          rememberSelectedWorkspace(workspace);
          router.push(workspaceHomeHref(workspace));
        }
        setOpen(false);
        onActionComplete?.();
        return;
      }
      const type = workspaceType(workspace);
      if (workspaceType(activeWorkspace) === "office" && type !== "office") {
        rememberLastOfficeWorkspace(activeWorkspace);
      }
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
      onActionComplete?.();
    },
    [activeId, activeWorkspace, router, setActiveWorkspace, officeEnabled, onActionComplete],
  );
  const handleNavigate = useCallback(
    (href: string) => {
      router.push(href);
      onActionComplete?.();
    },
    [router, onActionComplete],
  );

  return (
    <DropdownMenu modal={modal} open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <WorkspaceTrigger
          activeName={activeName}
          chevronTestId={chevronTestId}
          data-testid={triggerTestId}
          className={triggerClassName}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align={contentAlign} className={cn("w-72", contentClassName)}>
        <WorkspacePickerContent
          workspaces={workspaces.items}
          activeId={activeId}
          itemTestIdPrefix={itemTestIdPrefix}
          officeEnabled={officeEnabled}
          onWorkspaceSelect={handleSelect}
          onNavigate={handleNavigate}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
