"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { cn } from "@/lib/utils";
import {
  WorkspacePickerContent,
  WorkspaceTrigger,
  workspaceType,
  type WorkspaceItem,
} from "@/components/workspaces/workspace-picker-content";
import { workspaceHomeHref } from "./app-sidebar-workspace-navigation";
import { useSelectWorkspace } from "@/hooks/use-select-workspace";

/**
 * Compact, secondary workspace switcher inlined after the Kandev brand in the
 * sidebar header. Muted by default so the brand stays primary; the active
 * workspace name truncates with a small chevron hinting the dropdown. Only
 * rendered while the sidebar is expanded — the collapsed rail omits it.
 *
 * The trigger and the rows come from `workspace-picker-content`, shared with
 * the workspace settings switcher; only the selection behaviour below is this
 * surface's own.
 */

type WorkspacePickerProps = {
  triggerClassName?: string;
  contentClassName?: string;
  contentAlign?: "start" | "center" | "end";
  triggerTestId?: string;
  chevronTestId?: string;
  itemTestIdPrefix?: string;
  modal?: boolean;
  onActionComplete?: () => void;
  /**
   * Controlled open state. Omit for the default self-managed menu; the
   * sidebar-header instance passes the store flag so the global
   * WORKSPACE_PICKER shortcut can open it.
   */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

/**
 * Open state that is self-managed by default and controlled when the caller
 * passes `open`. `onOpenChange` fires in both modes, so a controlled owner sees
 * every dismissal (item select, outside click, Escape).
 *
 * A caller must pick one mode and keep it for the component's lifetime. Going
 * from a defined `open` to `undefined` (or back) silently swaps which state
 * wins and strands the other — the usual React controlled/uncontrolled
 * anti-pattern. Both current callers are static: the sidebar header always
 * passes the store flag, the mobile sheet never passes one.
 */
function useMenuOpenState(controlledOpen?: boolean, onOpenChange?: (open: boolean) => void) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const isControlled = controlledOpen !== undefined;
  const setOpen = useCallback(
    (next: boolean) => {
      if (!isControlled) setUncontrolledOpen(next);
      onOpenChange?.(next);
    },
    [isControlled, onOpenChange],
  );
  return { open: isControlled ? controlledOpen : uncontrolledOpen, setOpen };
}

function useControlledMenuFocus(controlledOpen: boolean | undefined) {
  const contentRef = useRef<HTMLDivElement>(null);
  const handleOpenAutoFocus = useCallback(
    (event: Event) => {
      if (controlledOpen === undefined) return;
      event.preventDefault();
      contentRef.current?.focus({ preventScroll: true });
    },
    [controlledOpen],
  );
  const handleEntryFocus = useCallback(
    (event: Event) => {
      if (controlledOpen !== undefined) event.preventDefault();
    },
    [controlledOpen],
  );
  return {
    ref: contentRef,
    onOpenAutoFocus: handleOpenAutoFocus,
    onEntryFocus: handleEntryFocus,
  };
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
  open: controlledOpen,
  onOpenChange,
}: WorkspacePickerProps = {}) {
  const { t } = useTranslation();
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const workspaces = useAppStore((s) => s.workspaces);
  const selectWorkspace = useSelectWorkspace();
  const resetKanbanWorkspaceContext = useAppStore((s) => s.resetKanbanWorkspaceContext);
  const { open, setOpen } = useMenuOpenState(controlledOpen, onOpenChange);

  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const activeId = activeWorkspace?.id ?? null;
  const activeName = activeWorkspace?.name ?? t("sidebar:workspaceFallback");
  const menuFocus = useControlledMenuFocus(controlledOpen);

  const handleSelect = useCallback(
    (workspace: WorkspaceItem) => {
      const { id } = workspace;
      if (id === activeId) {
        if (officeEnabled && workspaceType(workspace) === "kanban") {
          selectWorkspace(workspace);
          router.push(workspaceHomeHref(workspace));
        }
        setOpen(false);
        onActionComplete?.();
        return;
      }
      resetKanbanWorkspaceContext();
      selectWorkspace(workspace);
      if (workspaceType(workspace) === "kanban") {
        router.push(workspaceHomeHref(workspace));
      } else if (officeEnabled) {
        router.push(`/office?workspaceId=${id}`);
      }
      setOpen(false);
      onActionComplete?.();
    },
    [
      activeId,
      router,
      selectWorkspace,
      resetKanbanWorkspaceContext,
      officeEnabled,
      onActionComplete,
      setOpen,
    ],
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
          // The name fades in with the sidebar's expand animation; the shared
          // trigger stays animation-free for surfaces that never collapse.
          nameClassName="sidebar-fade-in"
          data-testid={triggerTestId}
          className={triggerClassName}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        {...menuFocus}
        align={contentAlign}
        className={cn("w-72", contentClassName)}
      >
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
