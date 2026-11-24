import { ApiError } from "@/lib/api/client";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  cancelPreview,
  getPreviewPolicy,
  preparePreview,
  readPreview,
  type PreviewSelection,
  type PreviewContext,
} from "@/lib/plugins/file-preview-client";
import type { RegisteredFileViewer } from "@/lib/plugins/registry";

let previewOwner: symbol | undefined;
export type PluginPreviewInput = {
  path: string;
  repo?: string;
  sessionId: string;
  workspaceId: string;
  viewer: RegisteredFileViewer;
};
type PreviewState = {
  status: "idle" | "loading" | "approval" | "ready" | "failed" | "busy" | "denied" | "too-large";
  selection?: PreviewSelection;
  bytes?: ArrayBuffer;
};

export function usePluginFilePreview(input: PluginPreviewInput) {
  const [state, setState] = useState<PreviewState>({ status: "idle" });
  const current = useRef<AbortController | null>(null);
  const cleanup = useRef<() => void>(() => {});
  const { path, repo, sessionId, workspaceId, viewer } = input;
  const { pluginId, id: viewerId, generation } = viewer;
  const release = useCallback(() => {
    cleanup.current();
    cleanup.current = () => {};
    current.current?.abort();
    current.current = null;
  }, []);
  const reset = useCallback(() => {
    release();
    setState({ status: "idle" });
  }, [release]);
  useEffect(() => {
    window.addEventListener("kandev-preview-policy-changed", reset);
    return () => window.removeEventListener("kandev-preview-policy-changed", reset);
  }, [reset]);
  const load = useCallback(
    async (selection: PreviewSelection, abort: AbortController) => {
      setState({ status: "loading", selection });
      try {
        const bytes = await readPreview({ pluginId, workspaceId }, selection, abort.signal);
        if (current.current === abort && !abort.signal.aborted)
          setState({ status: "ready", selection, bytes });
      } catch (error) {
        if (current.current === abort && !abort.signal.aborted) {
          cleanup.current();
          setState({ status: previewFailureStatus(error) });
        }
      }
    },
    [pluginId, workspaceId],
  );
  const request = useCallback(async () => {
    reset();
    if (previewOwner) {
      setState({ status: "busy" });
      return;
    }
    const owner = Symbol();
    previewOwner = owner;
    const abort = new AbortController();
    current.current = abort;
    let selection: PreviewSelection | undefined;
    let monitor: ReturnType<typeof setInterval> | undefined;
    const context = { pluginId, workspaceId };
    cleanup.current = () => {
      abort.abort();
      if (previewOwner === owner) previewOwner = undefined;
      if (monitor) clearInterval(monitor);
      if (selection) void cancelPreview(context, selection.id).catch(() => {});
    };
    setState({ status: "loading" });
    try {
      const policy = await getPreviewPolicy(context, abort.signal);
      if (abort.signal.aborted) return;
      selection = await preparePreview(
        context,
        { path, repo, session_id: sessionId, viewer_id: viewerId },
        abort.signal,
      );
      if (abort.signal.aborted) {
        void cancelPreview(context, selection.id).catch(() => {});
        return;
      }
      const selected = selection;
      monitor = monitorPreviewPolicy(context, abort.signal, policy.revision, () => {
        if (current.current === abort) reset();
      });
      if (selected.approval_required) setState({ status: "approval", selection: selected });
      else await load(selected, abort);
    } catch (error) {
      if (current.current === abort && !abort.signal.aborted) {
        cleanup.current();
        setState({ status: previewFailureStatus(error) });
      }
    }
  }, [reset, pluginId, workspaceId, path, repo, sessionId, viewerId, load]);
  const approve = useCallback(() => {
    if (state.status === "approval" && state.selection && current.current)
      void load(state.selection, current.current);
  }, [state, load]);
  const dismissApproval = useCallback(() => {
    if (state.status === "approval") reset();
  }, [state.status, reset]);
  useEffect(() => {
    void request();
    return release;
  }, [request, generation, release]);
  return { state, request, approve, dismissApproval };
}

function previewFailureStatus(error: unknown): PreviewState["status"] {
  if (error instanceof ApiError) {
    if (error.status === 413) return "too-large";
    if (error.status === 403) return "denied";
    if (error.status === 429) return "busy";
  }
  return "failed";
}

function monitorPreviewPolicy(
  context: PreviewContext,
  signal: AbortSignal,
  revision: number,
  cancel: () => void,
) {
  let checking = false;
  return setInterval(() => {
    if (checking || signal.aborted) return;
    checking = true;
    void getPreviewPolicy(context, signal)
      .then((next) => {
        if (!signal.aborted && next.revision !== revision) cancel();
      })
      .catch(() => {
        if (!signal.aborted) cancel();
      })
      .finally(() => {
        checking = false;
      });
  }, 2000);
}
