"use client";

import { useState, useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { getPlanLastSeen } from "@/lib/local-storage";
import { approveSessionAction } from "@/app/actions/workspaces";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { OpenFileTab } from "@/lib/types/backend";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";

export type SelectedDiff = {
  path: string;
  content?: string;
};

type UseSessionLayoutStateOptions = {
  sessionId?: string | null;
};

/** Handle approve action: call server action and trigger auto-start if configured. */
async function executeApprove(
  sessionId: string,
  taskId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  setTaskSession: (session: any) => void,
) {
  const response = await approveSessionAction(sessionId);
  if (response?.session) {
    setTaskSession(response.session);
  }
  if (
    response?.workflow_step?.events?.on_enter?.some(
      (a: { type: string }) => a.type === "auto_start_agent",
    )
  ) {
    const client = getWebSocketClient();
    if (client) {
      client.send({
        type: "request",
        action: "orchestrator.start",
        payload: {
          task_id: taskId,
          session_id: sessionId,
          workflow_step_id: response.workflow_step.id,
        },
      });
    }
  }
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

  const activeSession = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId] ?? null) : null,
  );
  const setTaskSession = useAppStore((state) => state.setTaskSession);

  // --- Agent state ---
  const isAgentWorking = activeSession?.state === "STARTING" || activeSession?.state === "RUNNING";

  const isPassthroughMode = useMemo(() => {
    if (!activeSession?.agent_profile_snapshot) return false;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const snapshot = activeSession.agent_profile_snapshot as any;
    return snapshot?.cli_passthrough === true;
  }, [activeSession?.agent_profile_snapshot]);

  // --- Diff selection state ---
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);

  const handleSelectDiff = useCallback((path: string, content?: string) => {
    setSelectedDiff({ path, content });
  }, []);
  const handleClearSelectedDiff = useCallback(() => {
    setSelectedDiff(null);
  }, []);

  // --- Open file request state ---
  const [openFileRequest, setOpenFileRequest] = useState<OpenFileTab | null>(null);

  const handleOpenFile = useCallback((file: OpenFileTab) => {
    setOpenFileRequest(file);
  }, []);
  const handleFileOpenHandled = useCallback(() => {
    setOpenFileRequest(null);
  }, []);

  // --- Git status for badges ---
  const gitStatus = useSessionGitStatus(effectiveSessionId);
  const { commits } = useSessionCommits(effectiveSessionId);

  const uncommittedCount = useMemo(() => {
    if (!gitStatus?.files) return 0;
    return Object.keys(gitStatus.files).length;
  }, [gitStatus]);

  const totalChangesCount = uncommittedCount + commits.length;

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
  const plan = useAppStore((state) =>
    activeTaskId ? state.taskPlans.byTaskId[activeTaskId] : null,
  );

  const hasUnseenPlanUpdate = useMemo(() => {
    // Don't show badge if we're viewing the plan
    if (!activeTaskId || !plan || currentMobilePanel === "plan") return false;
    if (plan.created_by !== "agent") return false;
    const lastSeen = getPlanLastSeen(activeTaskId);
    return plan.updated_at !== lastSeen;
  }, [activeTaskId, plan, currentMobilePanel]);

  // --- Approve button logic ---
  const showApproveButton =
    !!activeSession?.review_status && activeSession.review_status !== "approved" && !isAgentWorking;

  const handleApprove = useCallback(async () => {
    if (!effectiveSessionId || !activeTaskId) return;
    try {
      await executeApprove(effectiveSessionId, activeTaskId, setTaskSession);
    } catch (error) {
      console.error("Failed to approve session:", error);
    }
  }, [effectiveSessionId, activeTaskId, setTaskSession]);

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
    gitStatus,
    commits,
    uncommittedCount,
    totalChangesCount,

    // Plan
    plan,
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
