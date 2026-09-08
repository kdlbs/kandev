"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useRouter } from "@/lib/routing/client-router";
import {
  canvasHref,
  getCanvas,
  getCanvasRuntime,
  startCanvasEdit,
  type Canvas,
  type CanvasRuntimeResponse,
} from "@/lib/api/domains/canvas-api";
import { canvasErrorMessage } from "@/lib/api/domains/canvas-error-copy";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";
import { useCanvasHostCanvases } from "./canvas-host-picker";
import {
  CanvasDesktopActions,
  CanvasHostBody,
  CanvasHostDialogs,
  MobileCanvasActions,
  type CanvasHostState,
} from "./canvas-host-components";

function stateForCanvas(canvas: Canvas): CanvasHostState {
  if (canvas.status === "archived") return "archived";
  if (
    !canvas.active_release_id &&
    canvas.pending_release?.validation_status === "pending_permission"
  ) {
    return "pending_permission";
  }
  if (!canvas.active_release_id) return "pending_first_release";
  if (canvas.active_release_status === "pending_permission") return "pending_permission";
  if (canvas.active_release_status === "invalid") return "invalid_release";
  if (canvas.active_release_status === "unavailable") return "unavailable";
  return "loading_runtime";
}

function canvasAuthorityChanged(previous: Canvas, next: Canvas): boolean {
  return (
    previous.status !== next.status ||
    previous.plugin_id !== next.plugin_id ||
    previous.plugin_instance_id !== next.plugin_instance_id ||
    previous.scope_kind !== next.scope_kind ||
    previous.workspace_id !== next.workspace_id ||
    previous.task_id !== next.task_id ||
    previous.active_release_id !== next.active_release_id ||
    previous.active_release_status !== next.active_release_status ||
    previous.pending_release?.id !== next.pending_release?.id ||
    previous.pending_release?.validation_status !== next.pending_release?.validation_status ||
    previous.grant_generation !== next.grant_generation
  );
}

function useRuntimeRenewal() {
  const renewalTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const renewRuntimeRef = useRef<() => void>(() => undefined);

  const clearRuntimeRenewal = useCallback(() => {
    if (renewalTimerRef.current !== null) {
      clearTimeout(renewalTimerRef.current);
      renewalTimerRef.current = null;
    }
  }, []);

  const scheduleRuntimeRenewal = useCallback(
    (expiresInSeconds: number | undefined) => {
      clearRuntimeRenewal();
      if (!Number.isFinite(expiresInSeconds) || !expiresInSeconds || expiresInSeconds <= 0) {
        return;
      }
      const delay = Math.max(100, (expiresInSeconds - 30) * 1000);
      renewalTimerRef.current = setTimeout(() => {
        renewalTimerRef.current = null;
        renewRuntimeRef.current();
      }, delay);
    },
    [clearRuntimeRenewal],
  );

  return { clearRuntimeRenewal, scheduleRuntimeRenewal, renewRuntimeRef };
}

type CanvasHostRef<T> = { current: T };
type CanvasHostSetter<T> = (value: T | ((current: T) => T)) => void;

type CanvasHostSnapshotOptions = {
  canvasId: string;
  requestId: number;
  requestRef: CanvasHostRef<number>;
  canvasRef: CanvasHostRef<Canvas | null>;
  nextCanvas: Canvas;
  forceRuntimeReset: boolean;
  setCanvas: CanvasHostSetter<Canvas | null>;
  setRuntimeUrl: CanvasHostSetter<string | null>;
  setState: CanvasHostSetter<CanvasHostState>;
  setError: CanvasHostSetter<string | null>;
  clearRuntimeRenewal: () => void;
  applyRuntime: (runtime: CanvasRuntimeResponse) => void;
};

function applyCanvasRuntime(
  runtime: CanvasRuntimeResponse,
  setRuntimeUrl: CanvasHostSetter<string | null>,
  setState: CanvasHostSetter<CanvasHostState>,
  clearRuntimeRenewal: () => void,
  scheduleRuntimeRenewal: (expiresInSeconds: number | undefined) => void,
) {
  if (runtime.runtime_url) {
    setRuntimeUrl(runtime.runtime_url);
    setState("ready");
    scheduleRuntimeRenewal(runtime.expires_in_seconds);
  } else {
    clearRuntimeRenewal();
    setState("unavailable");
  }
}

type CanvasHostErrorOptions = {
  requestRef: CanvasHostRef<number>;
  requestId: number;
  reason: unknown;
  message: string;
  clearRuntimeRenewal: () => void;
  setState: CanvasHostSetter<CanvasHostState>;
  setError: CanvasHostSetter<string | null>;
};

function applyCanvasHostError(options: CanvasHostErrorOptions) {
  if (options.requestRef.current !== options.requestId) return;
  options.clearRuntimeRenewal();
  options.setState(
    typeof navigator !== "undefined" && !navigator.onLine ? "offline" : "unavailable",
  );
  options.setError(options.message || String(options.reason));
}

function applyCanvasSnapshot(options: CanvasHostSnapshotOptions): Promise<void> | undefined {
  if (options.requestRef.current !== options.requestId) return;
  const previousCanvas = options.canvasRef.current;
  options.canvasRef.current = options.nextCanvas;
  options.setCanvas(options.nextCanvas);
  if (
    !options.forceRuntimeReset &&
    previousCanvas &&
    !canvasAuthorityChanged(previousCanvas, options.nextCanvas)
  ) {
    return;
  }

  options.clearRuntimeRenewal();
  options.setRuntimeUrl(null);
  options.setError(null);
  const nextState = stateForCanvas(options.nextCanvas);
  options.setState(nextState);
  if (nextState !== "loading_runtime") return;
  return getCanvasRuntime(options.canvasId).then((runtime) => {
    if (options.requestRef.current !== options.requestId) return;
    options.applyRuntime(runtime);
  });
}

type CanvasHostRuntimeRequestOptions = {
  canvasId: string;
  requestId: number;
  requestRef: CanvasHostRef<number>;
  applyRuntime: (runtime: CanvasRuntimeResponse) => void;
  onError: (requestId: number, reason: unknown) => void;
  onComplete: () => void;
};

function requestCanvasHostRuntime(options: CanvasHostRuntimeRequestOptions) {
  getCanvasRuntime(options.canvasId)
    .then((runtime) => {
      if (options.requestRef.current !== options.requestId) return;
      options.applyRuntime(runtime);
    })
    .catch((reason: unknown) => options.onError(options.requestId, reason))
    .finally(options.onComplete);
}

type CanvasHostRequestOptions = {
  setCanvas: CanvasHostSetter<Canvas | null>;
  setRuntimeUrl: CanvasHostSetter<string | null>;
  setState: CanvasHostSetter<CanvasHostState>;
  setError: CanvasHostSetter<string | null>;
  requestRef: CanvasHostRef<number>;
  canvasRef: CanvasHostRef<Canvas | null>;
  renewingRef: CanvasHostRef<boolean>;
  clearRuntimeRenewal: () => void;
  scheduleRuntimeRenewal: (expiresInSeconds: number | undefined) => void;
};

function useCanvasHostRequests(canvasId: string, options: CanvasHostRequestOptions) {
  const { t } = useTranslation();
  const {
    setCanvas,
    setRuntimeUrl,
    setState,
    setError,
    requestRef,
    canvasRef,
    renewingRef,
    clearRuntimeRenewal,
    scheduleRuntimeRenewal,
  } = options;

  const applyRuntime = useCallback(
    (runtime: CanvasRuntimeResponse) =>
      applyCanvasRuntime(
        runtime,
        setRuntimeUrl,
        setState,
        clearRuntimeRenewal,
        scheduleRuntimeRenewal,
      ),
    [clearRuntimeRenewal, scheduleRuntimeRenewal],
  );

  const handleError = useCallback(
    (requestId: number, reason: unknown) =>
      applyCanvasHostError({
        requestRef,
        requestId,
        reason,
        message: canvasErrorMessage(reason, t, "canvases:loadFailed"),
        clearRuntimeRenewal,
        setState,
        setError,
      }),
    [clearRuntimeRenewal, t],
  );

  const applySnapshot = useCallback(
    (requestId: number, nextCanvas: Canvas, forceRuntimeReset: boolean) =>
      applyCanvasSnapshot({
        canvasId,
        requestId,
        requestRef,
        canvasRef,
        nextCanvas,
        forceRuntimeReset,
        setCanvas,
        setRuntimeUrl,
        setState,
        setError,
        clearRuntimeRenewal,
        applyRuntime,
      }),
    [applyRuntime, canvasId, clearRuntimeRenewal],
  );

  const renewRuntime = useCallback(() => {
    if (renewingRef.current) return;
    const requestId = ++requestRef.current;
    renewingRef.current = true;
    clearRuntimeRenewal();
    setState("loading_runtime");
    setError(null);
    requestCanvasHostRuntime({
      canvasId,
      requestId,
      requestRef,
      applyRuntime,
      onError: handleError,
      onComplete: () => {
        renewingRef.current = false;
      },
    });
  }, [applyRuntime, canvasId, clearRuntimeRenewal, handleError]);

  const load = useCallback(() => {
    const requestId = ++requestRef.current;
    clearRuntimeRenewal();
    canvasRef.current = null;
    setCanvas(null);
    setRuntimeUrl(null);
    setState("loading_metadata");
    setError(null);
    getCanvas(canvasId)
      .then((nextCanvas) => applySnapshot(requestId, nextCanvas, true))
      .catch((reason: unknown) => handleError(requestId, reason));
  }, [applySnapshot, canvasId, clearRuntimeRenewal, handleError]);

  const refresh = useCallback(() => {
    const requestId = ++requestRef.current;
    getCanvas(canvasId)
      .then((nextCanvas) => applySnapshot(requestId, nextCanvas, false))
      .catch((reason: unknown) => handleError(requestId, reason));
  }, [applySnapshot, canvasId, handleError]);

  return { load, refresh, renewRuntime };
}

function useCanvasHost(canvasId: string) {
  const [canvas, setCanvas] = useState<Canvas | null>(null);
  const [runtimeUrl, setRuntimeUrl] = useState<string | null>(null);
  const [state, setState] = useState<CanvasHostState>("loading_metadata");
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);
  const canvasRef = useRef<Canvas | null>(null);
  const renewingRef = useRef(false);
  const { clearRuntimeRenewal, scheduleRuntimeRenewal, renewRuntimeRef } = useRuntimeRenewal();
  const lifecycleRevision = useCanvasLifecycleRevision();
  const { load, refresh, renewRuntime } = useCanvasHostRequests(canvasId, {
    setCanvas,
    setRuntimeUrl,
    setState,
    setError,
    requestRef,
    canvasRef,
    renewingRef,
    clearRuntimeRenewal,
    scheduleRuntimeRenewal,
  });

  useEffect(() => {
    renewRuntimeRef.current = renewRuntime;
    return () => {
      renewRuntimeRef.current = () => undefined;
    };
  }, [renewRuntime]);

  useEffect(() => {
    load();
    return () => {
      requestRef.current += 1;
      clearRuntimeRenewal();
    };
  }, [clearRuntimeRenewal, load]);

  useEffect(() => {
    if (!canvasRef.current) return;
    refresh();
  }, [lifecycleRevision, refresh]);

  return {
    canvas,
    runtimeUrl,
    state,
    error,
    lifecycleRevision,
    load,
    renewRuntime,
    setHostError: setError,
  };
}

export function CanvasHostRoute({ canvasId }: { canvasId: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();
  const { canvas, runtimeUrl, state, error, load, renewRuntime, setHostError } =
    useCanvasHost(canvasId);
  const hostCanvases = useCanvasHostCanvases(canvas);
  const [menuOpen, setMenuOpen] = useState(false);
  const [promotionOpen, setPromotionOpen] = useState(false);
  const [releasesOpen, setReleasesOpen] = useState(false);
  const [editing, setEditing] = useState(false);

  const edit = async () => {
    if (!canvas) return;
    setEditing(true);
    try {
      const response = await startCanvasEdit(canvas.id);
      if (response.task_id) {
        const query = response.session_id
          ? `?sessionId=${encodeURIComponent(response.session_id)}`
          : "";
        router.push(`/t/${encodeURIComponent(response.task_id)}${query}`);
      }
    } catch (reason: unknown) {
      setHostError(canvasErrorMessage(reason, t, "canvases:actionFailed"));
    } finally {
      setEditing(false);
      setMenuOpen(false);
    }
  };

  const selectCanvas = useCallback(
    (nextCanvas: Canvas) => {
      setMenuOpen(false);
      if (nextCanvas.id !== canvasId) router.push(canvasHref(nextCanvas.id));
    },
    [canvasId, router],
  );

  const title = canvas?.title || t("canvases:canvas");
  const desktopActions = canvas ? (
    <CanvasDesktopActions
      canvas={canvas}
      editing={editing}
      onEdit={() => void edit()}
      onPromote={() => setPromotionOpen(true)}
      onReleases={() => setReleasesOpen(true)}
    />
  ) : null;

  return (
    <PageShell
      title={title}
      backHref="/"
      backLabel={t("sidebar:home")}
      scroll="none"
      actions={!isMobile ? desktopActions : undefined}
      contentTestId="canvas-route-content"
      showNavTrigger
    >
      <CanvasHostBody
        canvasId={canvasId}
        title={title}
        state={state}
        isMobile={isMobile}
        menuOpen={menuOpen}
        runtimeUrl={runtimeUrl}
        error={error}
        onOpenActions={() => setMenuOpen(true)}
        onRuntimeError={() => void renewRuntime()}
        onRetry={load}
      />
      <MobileCanvasActions
        canvas={canvas}
        open={menuOpen}
        onOpenChange={setMenuOpen}
        onEdit={() => void edit()}
        onPromote={() => setPromotionOpen(true)}
        onReleases={() => setReleasesOpen(true)}
        editing={editing}
        canvases={hostCanvases}
        onSelectCanvas={selectCanvas}
      />
      <CanvasHostDialogs
        canvas={canvas}
        promotionOpen={promotionOpen}
        onPromotionOpenChange={setPromotionOpen}
        releasesOpen={releasesOpen}
        onReleasesOpenChange={setReleasesOpen}
        onPromotionCompleted={() => router.push(canvas ? canvasHref(canvas.id) : "/")}
        onChanged={load}
      />
    </PageShell>
  );
}
