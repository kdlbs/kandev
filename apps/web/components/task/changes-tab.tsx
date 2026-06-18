"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  DockviewDefaultTab,
  type DockviewApi,
  type IDockviewPanelHeaderProps,
} from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionChangesCount } from "@/hooks/domains/session/use-session-changes-count";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { cn } from "@kandev/ui/lib/utils";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";

type DockviewPanel = NonNullable<ReturnType<DockviewApi["getPanel"]>>;

function groupContainsAgentSessionPanel(panel: DockviewPanel): boolean {
  return panel.group.panels.some((p) => p.id === "chat" || p.id.startsWith("session:"));
}

/** Auto-activate the changes panel unless it shares a group with agent sessions. */
function autoActivateChangesPanel(): void {
  const { api } = useDockviewStore.getState();
  if (!api) return;

  const panel = api.getPanel("changes");
  if (!panel || groupContainsAgentSessionPanel(panel)) return;
  panel.api.setActive();
}

/**
 * Custom tab component for the Changes panel.
 * Provides auto-activation, flash animation on new changes,
 * and a badge showing unseen change count.
 */
export function ChangesTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;
  const onDoubleClick = useTabMaximizeOnDoubleClick(api);

  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const totalCount = useSessionChangesCount(activeSessionId ?? null);

  // gitStatus is undefined until the first WS git-status event arrives,
  // which marks the end of the initial data load for this session.
  const gitStatusLoaded = gitStatus !== undefined;

  const prevTotalRef = useRef(totalCount);
  const seenCountRef = useRef(api.isActive ? totalCount : 0);
  const activeSessionRef = useRef(activeSessionId);
  const flashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Armed once we know the initial git data has settled. Until then, any
  // 0→N transition is treated as an initial load, not a real new change.
  const initializedRef = useRef(false);

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
    if (activeSessionRef.current !== activeSessionId) {
      activeSessionRef.current = activeSessionId;
      prevTotalRef.current = totalCount;
      seenCountRef.current = api.isActive ? totalCount : 0;
      initializedRef.current = false;
      setBadgeCount(0);
    }

    if (api.isActive) {
      seenCountRef.current = totalCount;
    }

    const prev = prevTotalRef.current;
    prevTotalRef.current = totalCount;

    const increased = totalCount > prev && totalCount > 0;
    const decreased = totalCount < prev;

    // Auto-activate on real post-load updates, but only after initial git data
    // has settled. gitStatusLoaded is false until the first WS git-status event
    // arrives, guaranteeing existing changes on page refresh do not steal focus.
    if (!initializedRef.current) {
      if (gitStatusLoaded) initializedRef.current = true;
    } else if (increased) {
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
  }, [totalCount, api, gitStatusLoaded, activeSessionId]);

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
      <ContextMenuTrigger
        className="flex h-full items-center cursor-pointer select-none"
        onDoubleClick={onDoubleClick}
      >
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
