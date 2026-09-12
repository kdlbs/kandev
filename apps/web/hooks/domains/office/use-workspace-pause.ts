"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import {
  getWorkspacePause,
  postWorkspacePause,
  postWorkspaceResume,
  type WorkspacePauseResponse,
} from "@/lib/api/domains/office-pause-api";
import type { WorkspacePauseRecord, WorkspacePauseStatus } from "@/lib/state/slices/office/types";
import { t } from "@/lib/i18n";

export type WorkspacePauseActionResult = { ok: true } | { ok: false; error?: string };

export type UseWorkspacePauseResult = {
  record: WorkspacePauseRecord | null;
  status: WorkspacePauseStatus;
  isPaused: boolean;
  isMutating: boolean;
  refresh: () => Promise<void>;
  pause: (reason: string) => Promise<WorkspacePauseActionResult>;
  resume: (reason?: string) => Promise<WorkspacePauseActionResult>;
};

type MutateAction = (workspaceId: string, reason: string) => Promise<WorkspacePauseResponse>;

/**
 * Implements the workspace kill switch's "Frontend state" input table
 * (docs/specs/office/system-design/workspace-kill-switch-02.md): mount and
 * workspace-change reset+read, WS-reconnect read, and pause/resume mutations
 * — every response funneled through the store's `applyPauseResponse` guard
 * so a superseded response (wrong workspace, or an older request tag) is
 * discarded rather than clobbering fresher state.
 */
export function useWorkspacePause(workspaceId: string | null): UseWorkspacePauseResult {
  const storeApi = useAppStoreApi();
  const record = useAppStore((s) => s.office.pause.record);
  const status = useAppStore((s) => s.office.pause.status);
  const beginPauseRequest = useAppStore((s) => s.beginPauseRequest);
  const resetPauseState = useAppStore((s) => s.resetPauseState);
  const applyPauseResponse = useAppStore((s) => s.applyPauseResponse);
  const [isMutating, setIsMutating] = useState(false);

  const read = useCallback(async () => {
    if (!workspaceId) return;
    const tag = beginPauseRequest();
    try {
      const res = await getWorkspacePause(workspaceId);
      const activeWorkspaceId = storeApi.getState().workspaces.activeId;
      applyPauseResponse(tag, res.workspaceId, activeWorkspaceId, {
        kind: "read-success",
        paused: res.paused,
        record: res.record,
      });
    } catch {
      const activeWorkspaceId = storeApi.getState().workspaces.activeId;
      applyPauseResponse(tag, workspaceId, activeWorkspaceId, { kind: "read-failure" });
    }
  }, [workspaceId, beginPauseRequest, applyPauseResponse, storeApi]);

  // Mount, and every change of selected workspace: status `unknown`, clear
  // the record, then issue a read. `read` is intentionally omitted from the
  // dependency array — it is recreated whenever workspaceId changes, which
  // this effect already depends on directly.
  useEffect(() => {
    if (!workspaceId) return;
    resetPauseState();
    void read();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, resetPauseState]);

  // WS reconnect: issue a read; status stays whatever it was, since the last
  // known state remains the best available until the read answers.
  const connectionStatus = useAppStore((s) => s.connection.status);
  const wasConnected = useRef(connectionStatus === "connected");
  useEffect(() => {
    const justReconnected = connectionStatus === "connected" && !wasConnected.current;
    wasConnected.current = connectionStatus === "connected";
    if (justReconnected) void read();
  }, [connectionStatus, read]);

  const mutate = useCallback(
    async (action: MutateAction, reason: string): Promise<WorkspacePauseActionResult> => {
      if (!workspaceId) return { ok: false, error: t("office:pauseUnavailable") };
      const tag = beginPauseRequest();
      setIsMutating(true);
      try {
        const res = await action(workspaceId, reason);
        const activeWorkspaceId = storeApi.getState().workspaces.activeId;
        applyPauseResponse(tag, res.workspaceId, activeWorkspaceId, {
          kind: "mutate-success",
          paused: res.paused,
          record: res.record,
        });
        return { ok: true };
      } catch (err) {
        const activeWorkspaceId = storeApi.getState().workspaces.activeId;
        // Whether the guard applies this outcome to the shared store is a
        // separate question from whether this specific caller's request
        // failed: this call genuinely errored, so its direct caller (still
        // awaiting this promise, e.g. an open dialog) must learn that,
        // regardless of whether a newer request or a workspace switch means
        // the shared store no longer reflects it.
        applyPauseResponse(tag, workspaceId, activeWorkspaceId, { kind: "mutate-failure" });
        const message = err instanceof ApiError ? err.message : t("office:pauseRequestFailed");
        return { ok: false, error: message };
      } finally {
        setIsMutating(false);
      }
    },
    [workspaceId, beginPauseRequest, applyPauseResponse, storeApi],
  );

  const pause = useCallback((reason: string) => mutate(postWorkspacePause, reason), [mutate]);
  const resume = useCallback(
    (reason: string = "") => mutate(postWorkspaceResume, reason),
    [mutate],
  );

  return {
    record,
    status,
    isPaused: record !== null,
    isMutating,
    refresh: read,
    pause,
    resume,
  };
}
