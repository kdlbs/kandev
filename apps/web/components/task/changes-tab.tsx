"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { focusOrAddPanel } from "@/lib/state/dockview-layout-builders";
import { cn } from "@kandev/ui/lib/utils";

/** Auto-activate the changes panel in the right sidebar, or quietly add to center if missing. */
function autoActivateChangesPanel(): void {
  const { api, rightTopGroupId, centerGroupId } = useDockviewStore.getState();
  if (!api) return;

  const panel = api.getPanel("changes");
  if (panel && panel.group.id === rightTopGroupId) {
    panel.api.setActive();
    return;
  }

  if (!panel) {
    focusOrAddPanel(
      api,
      {
        id: "changes",
        component: "changes",
        tabComponent: "changesTab",
        title: "Changes",
        position: { referenceGroup: centerGroupId },
      },
      true,
    );
  }
}

/**
 * Custom tab component for the Changes panel.
 * Provides auto-activation, flash animation on new changes,
 * and a badge showing unseen change count.
 */
export function ChangesTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;

  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const fileCount = gitStatus?.files ? Object.keys(gitStatus.files).length : 0;
  const totalCount = fileCount + commits.length;

  const prevTotalRef = useRef(totalCount);
  const seenCountRef = useRef(api.isActive ? totalCount : 0);
  const flashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [isFlashing, setIsFlashing] = useState(false);
  const [badgeCount, setBadgeCount] = useState(0);

  // Reset seenCount when the user activates this tab
  useEffect(() => {
    const disposable = api.onDidActiveChange((event) => {
      if (event.isActive) {
        seenCountRef.current = totalCount;
        setBadgeCount(0);
      }
    });
    return () => disposable.dispose();
  }, [api, totalCount]);

  // React to totalCount changes: auto-activate, flash, badge
  useEffect(() => {
    if (api.isActive) {
      seenCountRef.current = totalCount;
    }

    const prev = prevTotalRef.current;
    prevTotalRef.current = totalCount;

    const increased = totalCount > prev && totalCount > 0;
    const decreased = totalCount < prev;

    if (increased && prev === 0) {
      autoActivateChangesPanel();
    }

    if (increased) {
      if (flashTimerRef.current) clearTimeout(flashTimerRef.current);
      // Defer setState to satisfy react-hooks/set-state-in-effect
      flashTimerRef.current = setTimeout(() => setIsFlashing(false), 1000);
      requestAnimationFrame(() => setIsFlashing(true));
    }

    if ((increased || decreased) && !api.isActive) {
      const unseen = Math.max(0, totalCount - seenCountRef.current);
      requestAnimationFrame(() => setBadgeCount(unseen));
    }
  }, [totalCount, api]);

  // Cleanup flash timer on unmount
  useEffect(() => {
    return () => {
      if (flashTimerRef.current) clearTimeout(flashTimerRef.current);
    };
  }, []);

  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter(
      (p) => p.id !== api.id && p.id !== "chat" && !p.id.startsWith("session:"),
    );
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);

  return (
    <ContextMenu>
      <ContextMenuTrigger className="flex h-full items-center">
        <div className={cn("relative", isFlashing && "animate-changes-flash")}>
          <DockviewDefaultTab {...props} />
          {badgeCount > 0 && (
            <span className="absolute top-0.5 left-0 size-2 rounded-full bg-primary pointer-events-none" />
          )}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem className="cursor-pointer" onSelect={handleCloseOthers}>
          Close Others
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
