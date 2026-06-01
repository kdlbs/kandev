"use client";

import { useState, useCallback, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { taskPlanQueryOptions } from "@/lib/query/query-options/session";
import { useSessionChangesCount } from "@/hooks/domains/session/use-session-changes-count";
import { useTaskSessionById } from "@/hooks/domains/session/use-task-session-by-id";
import { getPlanLastSeen } from "@/lib/local-storage";
import { executeApprove } from "@/lib/services/session-approve";
import type { OpenFileTab } from "@/lib/types/backend";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";

export type SelectedDiff = {
  path: string;
  content?: string;
};

type UseSessionLayoutStateOptions = {
  sessionId?: string | null;
};

function useSelectedDiffState() {
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const handleSelectDiff = useCallback((path: string, content?: string) => {
    setSelectedDiff({ path, content });
  }, []);
  const handleClearSelectedDiff = useCallback(() => {
    setSelectedDiff(null);
  }, []);

  return { selectedDiff, handleSelectDiff, handleClearSelectedDiff };
}

function useOpenFileRequestState() {
  const [openFileRequest, setOpenFileRequest] = useState<OpenFileTab | null>(null);
  const handleOpenFile = useCallback((file: OpenFileTab) => {
    setOpenFileRequest(file);
  }, []);
  const handleFileOpenHandled = useCallback(() => {
    setOpenFileRequest(null);
  }, []);

  return { openFileRequest, handleOpenFile, handleFileOpenHandled };
}

/**
 * Whether the active task's plan has an unseen agent update, for the mobile
 * plan badge. The plan is read from the TanStack Query cache; the last-seen
 * marker is a client-only value in localStorage.
 */
function useUnseenPlanBadge(activeTaskId: string | null, isViewingPlan: boolean): boolean {
  const planQuery = useQuery({
    ...taskPlanQueryOptions(activeTaskId ?? ""),
    enabled: !!activeTaskId,
  });
  const plan = planQuery.data?.plan ?? null;

  return useMemo(() => {
    if (!activeTaskId || !plan || isViewingPlan) return false;
    if (plan.created_by !== "agent") return false;
    return plan.updated_at !== getPlanLastSeen(activeTaskId);
  }, [activeTaskId, plan, isViewingPlan]);
}

/**
 * Shared hook for session layout state used across mobile, tablet, and desktop layouts.
 * Consolidates common state and logic to avoid duplication.
 */
export function useSessionLayoutState(options: UseSessionLayoutStateOptions = {}) {
  const { sessionId = null } = options;

  // --- Core session state ---
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const effectiveSessionId = activeSessionId ?? sessionId ?? null;
  const sessionKey = effectiveSessionId ?? "";

  const activeSession = useTaskSessionById(effectiveSessionId);
  const queryClient = useQueryClient();

  // --- Agent state ---
  const isAgentWorking = activeSession?.state === "STARTING" || activeSession?.state === "RUNNING";

  const isPassthroughMode = useMemo(() => isPassthroughSession(activeSession), [activeSession]);

  const { selectedDiff, handleSelectDiff, handleClearSelectedDiff } = useSelectedDiffState();
  const { openFileRequest, handleOpenFile, handleFileOpenHandled } = useOpenFileRequestState();

  // --- Git status for badges ---
  // `useSessionChangesCount` reads the per-repo statuses so the badge count
  // stays correct in multi-repo workspaces and doesn't flicker as sibling
  // repos overwrite the legacy single-status map.
  const totalChangesCount = useSessionChangesCount(effectiveSessionId);

  // --- Mobile session state (computed before plan badge to use in badge logic) ---
  const activePanelBySessionId = useAppStore((state) => state.mobileSession.activePanelBySessionId);
  const isTaskSwitcherOpen = useAppStore((state) => state.mobileSession.isTaskSwitcherOpen);
  const setMobileSessionPanel = useAppStore((state) => state.setMobileSessionPanel);
  const setMobileSessionTaskSwitcherOpen = useAppStore(
    (state) => state.setMobileSessionTaskSwitcherOpen,
  );

  const currentMobilePanel: MobileSessionPanel = effectiveSessionId
    ? (activePanelBySessionId[effectiveSessionId] ?? "chat")
    : "chat";

  // --- Plan badge ---
  const hasUnseenPlanUpdate = useUnseenPlanBadge(activeTaskId, currentMobilePanel === "plan");

  // --- Approve button logic ---
  const showApproveButton =
    !!activeSession?.review_status && activeSession.review_status !== "approved" && !isAgentWorking;

  const handleApprove = useCallback(async () => {
    if (!effectiveSessionId || !activeTaskId) return;
    try {
      await executeApprove(effectiveSessionId, activeTaskId, queryClient);
    } catch (error) {
      console.error("Failed to approve session:", error);
    }
  }, [effectiveSessionId, activeTaskId, queryClient]);

  const handlePanelChange = useCallback(
    (panel: MobileSessionPanel) => {
      if (effectiveSessionId) {
        setMobileSessionPanel(effectiveSessionId, panel);
      }
    },
    [effectiveSessionId, setMobileSessionPanel],
  );

  const handleMenuClick = useCallback(() => {
    setMobileSessionTaskSwitcherOpen(true);
  }, [setMobileSessionTaskSwitcherOpen]);

  return {
    // Core session
    activeTaskId,
    activeSessionId,
    effectiveSessionId,
    sessionKey,
    activeSession,

    // Agent state
    isAgentWorking,
    isPassthroughMode,

    // Diff selection
    selectedDiff,
    handleSelectDiff,
    handleClearSelectedDiff,

    // File open
    openFileRequest,
    handleOpenFile,
    handleFileOpenHandled,

    // Git status
    totalChangesCount,

    // Plan
    hasUnseenPlanUpdate,

    // Approve
    showApproveButton,
    handleApprove,

    // Mobile session panel
    currentMobilePanel,
    handlePanelChange,
    isTaskSwitcherOpen,
    handleMenuClick,
    setMobileSessionTaskSwitcherOpen,
  };
}
