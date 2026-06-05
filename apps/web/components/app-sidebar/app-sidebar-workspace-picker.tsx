"use client";

import { forwardRef, useCallback, useState, type ComponentPropsWithoutRef } from "react";
import { useRouter } from "next/navigation";
import { IconCheck, IconChevronDown, IconPlus } from "@tabler/icons-react";
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

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <WorkspaceTrigger activeName={activeName} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-60">
        {workspaces.items.length === 0 ? (
          <DropdownMenuItem disabled>No workspaces</DropdownMenuItem>
        ) : (
          workspaces.items.map((ws) => (
            <DropdownMenuItem
              key={ws.id}
              data-testid={`sidebar-workspace-item-${ws.id}`}
              onSelect={() => handleSelect(ws.id)}
              className="cursor-pointer gap-2"
            >
              <span className="flex-1 truncate">{ws.name}</span>
              {ws.id === activeId && <IconCheck className="h-3.5 w-3.5" />}
            </DropdownMenuItem>
          ))
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          onSelect={() => {
            router.push("/office/setup?mode=new");
          }}
        >
          <IconPlus className="h-3.5 w-3.5" />
          <span>Add workspace</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
