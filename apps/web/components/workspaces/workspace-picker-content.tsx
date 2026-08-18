"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { useTranslation } from "react-i18next";
import { IconBriefcase, IconChevronDown, IconLayoutKanban, IconPlus } from "@tabler/icons-react";
import { DropdownMenuItem, DropdownMenuSeparator } from "@kandev/ui/dropdown-menu";
import { ActiveWorkspaceBadge } from "@/components/settings/record-badges";
import { cn } from "@/lib/utils";

/**
 * The one workspace switcher design, shared by the sidebar header picker and
 * the workspace settings pages. Both surfaces draw the same trigger and the
 * same rows — a switcher that reads differently depending on where it is
 * opened is worse than two obviously different controls.
 *
 * Only behaviour differs: the sidebar picker changes the active workspace,
 * while the settings switcher navigates between settings pages. That is why
 * selection is a callback here rather than logic baked into the rows.
 */

export type WorkspaceType = "kanban" | "office";

export type WorkspaceItem = {
  id: string;
  name: string;
  office_workflow_id?: string | null;
};

export function workspaceType(workspace: WorkspaceItem | undefined): WorkspaceType {
  return workspace?.office_workflow_id ? "office" : "kanban";
}

/** Returns a catalog key, not copy: this is a plain function with no hook. */
function workspaceTypeLabelKey(type: WorkspaceType) {
  return type === "office" ? "sidebar:office" : "sidebar:kanban";
}

function WorkspaceTypeIcon({ type, className }: { type: WorkspaceType; className: string }) {
  if (type === "office") {
    return <IconBriefcase className={className} />;
  }
  return <IconLayoutKanban className={className} />;
}

/**
 * The switcher's button: a bordered control carrying the current workspace
 * name and a chevron. `nameClassName` exists for the sidebar's expand
 * animation (`sidebar-fade-in`), which must not leak into the settings header.
 *
 * forwardRef + prop spread so `DropdownMenuTrigger asChild` can wire the
 * trigger (ref, onClick, aria-*, data-state) onto the underlying button.
 */
export const WorkspaceTrigger = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<"button"> & {
    activeName: string;
    chevronTestId?: string;
    nameClassName?: string;
  }
>(function WorkspaceTrigger(
  { activeName, chevronTestId, nameClassName, className, ...props },
  ref,
) {
  const { t } = useTranslation();
  return (
    <button
      ref={ref}
      type="button"
      aria-label={t("sidebar:switchWorkspace")}
      className={cn(
        "group/ws flex h-7 min-w-0 flex-1 items-center gap-1.5 rounded-md border border-border/70 bg-background px-2.5 text-sm font-medium text-foreground shadow-sm cursor-pointer transition-colors hover:border-border hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        className,
      )}
      {...props}
    >
      <span className={cn("min-w-0 flex-1 truncate text-left", nameClassName)}>{activeName}</span>
      <IconChevronDown
        data-testid={chevronTestId}
        className="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-colors group-hover/ws:text-foreground/80"
      />
    </button>
  );
});

export type WorkspacePickerContentProps = {
  workspaces: WorkspaceItem[];
  activeId: string | null;
  itemTestIdPrefix: string;
  officeEnabled: boolean;
  onWorkspaceSelect: (workspace: WorkspaceItem) => void;
  onNavigate: (href: string) => void;
};

export function WorkspacePickerContent({
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
  const { t } = useTranslation();
  if (workspaces.length === 0) {
    return <DropdownMenuItem disabled>{t("sidebar:noWorkspaces")}</DropdownMenuItem>;
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
        {/* The same "Active" pill the settings menu and the workspace list
            draw, sitting with the type badge in the row's qualifier column
            rather than splitting the icon from the name. */}
        {ws.id === activeId && <ActiveWorkspaceBadge />}
        <span className="shrink-0 rounded border border-border/60 px-1.5 py-0.5 text-[10px] font-medium leading-none text-muted-foreground">
          {t(workspaceTypeLabelKey(type))}
        </span>
      </DropdownMenuItem>
    );
  });
}

function WorkspaceCreateItems({
  officeEnabled,
  onNavigate,
}: Pick<WorkspacePickerContentProps, "officeEnabled" | "onNavigate">) {
  const { t } = useTranslation();
  if (!officeEnabled) {
    return (
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/settings/workspaces")}
      >
        <IconPlus className="h-3.5 w-3.5" />
        <span>{t("sidebar:addWorkspace")}</span>
      </DropdownMenuItem>
    );
  }

  return (
    <>
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/settings/workspaces")}
      >
        <IconLayoutKanban className="h-3.5 w-3.5" />
        <span>{t("sidebar:newKanbanWorkspace")}</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => onNavigate("/office/setup?mode=new")}
      >
        <IconBriefcase className="h-3.5 w-3.5" />
        <span>{t("sidebar:newOfficeWorkspace")}</span>
      </DropdownMenuItem>
    </>
  );
}
