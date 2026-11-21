"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type ReactNode,
  type SetStateAction,
} from "react";
import { createPortal } from "react-dom";
import { IconAlertTriangle, IconCheck, IconInfoCircle, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

import { ApiError } from "@/lib/api/client";
import type { SessionConfigurationResult } from "@/lib/api/domains/session-api";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import { cn } from "@/lib/utils";

export type ConfigurationChangeOutcome = {
  configId?: string;
  label: string;
  requested: string;
  result?: SessionConfigurationResult;
};

type FeedbackPosition = { left: number; top: number; maxHeight: number };

function updateMobileFeedbackPosition(
  anchor: HTMLElement,
  message: HTMLDivElement | null,
  setPosition: Dispatch<SetStateAction<FeedbackPosition | null>>,
) {
  const bounds = anchor.getBoundingClientRect();
  const viewport = window.visualViewport;
  const gutter = 8;
  const viewportLeft = viewport?.offsetLeft ?? 0;
  const viewportTop = viewport?.offsetTop ?? 0;
  const viewportWidth = viewport?.width ?? window.innerWidth;
  const viewportHeight = viewport?.height ?? window.innerHeight;
  const maxWidth = Math.min(416, viewportWidth * 0.9);
  const maxHeight = Math.max(80, viewportHeight - gutter * 2);
  const contentHeight = Math.min(message?.getBoundingClientRect().height ?? maxHeight, maxHeight);
  const left = Math.max(
    viewportLeft + gutter,
    Math.min(bounds.left, viewportLeft + viewportWidth - maxWidth - gutter),
  );
  const top = Math.max(
    viewportTop + gutter,
    Math.min(bounds.top - contentHeight - 4, viewportTop + viewportHeight - contentHeight - gutter),
  );
  setPosition({ left, top, maxHeight });
}

type FeedbackStatus =
  | "applied"
  | "clamped"
  | "saved_for_launch"
  | "unconfirmed"
  | "rejected"
  | "persistence_failed"
  | "mixed"
  | "partial";

export type PickerConfigurationFeedbackScope = {
  sessionId: string | null;
  executionId?: string;
};

type PickerFeedback = {
  id: number;
  status: FeedbackStatus;
  changes: ConfigurationChangeOutcome[];
  failed?: { label: string; value: string };
  reason?: string;
  retry?: () => void;
  scope?: PickerConfigurationFeedbackScope;
};

export function PickerConfigurationFeedbackMessage<T extends HTMLElement>({
  anchorRef,
  isMobile,
  message,
  passive,
}: {
  anchorRef: RefObject<T | null>;
  isMobile: boolean;
  message: ReactNode;
  passive: boolean;
}) {
  const [position, setPosition] = useState<FeedbackPosition | null>(null);
  const messageRef = useRef<HTMLDivElement>(null);
  const portalTarget = typeof document === "undefined" ? null : document.body;

  useLayoutEffect(() => {
    if (!isMobile || !message) {
      setPosition(null);
      return;
    }

    const anchor = anchorRef.current;
    if (!anchor) return;

    const updatePosition = () =>
      updateMobileFeedbackPosition(anchor, messageRef.current, setPosition);

    updatePosition();
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    window.visualViewport?.addEventListener("resize", updatePosition);
    window.visualViewport?.addEventListener("scroll", updatePosition);
    const observer = new ResizeObserver(updatePosition);
    observer.observe(anchor);
    if (messageRef.current) observer.observe(messageRef.current);
    return () => {
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
      window.visualViewport?.removeEventListener("resize", updatePosition);
      window.visualViewport?.removeEventListener("scroll", updatePosition);
      observer.disconnect();
    };
  }, [anchorRef, isMobile, Boolean(message)]);

  if (!message) return null;
  if (!isMobile) {
    return (
      <div
        className={cn("absolute bottom-full left-0 z-40 mb-1", passive && "pointer-events-none")}
      >
        {message}
      </div>
    );
  }
  if (!portalTarget || !position) return null;

  return createPortal(
    <div
      className={cn("fixed z-40", passive && "pointer-events-none")}
      ref={messageRef}
      style={{
        left: position.left,
        top: position.top,
        maxWidth: "min(26rem, 90vw)",
        maxHeight: position.maxHeight,
        overflowY: "auto",
        overscrollBehavior: "contain",
      }}
    >
      {message}
    </div>,
    portalTarget,
  );
}

export type PickerConfigurationFeedbackController = {
  begin: () => number;
  showResult: (
    id: number,
    label: string,
    requested: string,
    result?: SessionConfigurationResult,
  ) => void;
  showError: (id: number, label: string, requested: string, error: unknown) => void;
  showChanges: (
    id: number,
    changes: ConfigurationChangeOutcome[],
    failure?: { label: string; value: string; error?: unknown },
    retry?: () => void,
  ) => void;
};

function errorResult(error: unknown): SessionConfigurationResult | undefined {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object")
    return undefined;
  const result = (error.body as { configuration_result?: unknown }).configuration_result;
  return result && typeof result === "object" ? (result as SessionConfigurationResult) : undefined;
}

function isScopedConfirmation(
  result: SessionConfigurationResult,
  scope?: PickerConfigurationFeedbackScope,
): result is SessionConfigurationResult & { effective: string } {
  return Boolean(
    result.confirmed === true &&
    result.effective &&
    scope?.sessionId &&
    scope.executionId &&
    result.execution_id === scope.executionId,
  );
}

function resultStatus(
  result: SessionConfigurationResult | undefined,
  scope?: PickerConfigurationFeedbackScope,
): FeedbackStatus {
  if (result?.status === "saved_for_launch") return "saved_for_launch";
  if (result?.status === "rejected") return "rejected";
  if (!result || !isScopedConfirmation(result, scope)) return "unconfirmed";
  if (result.status === "persistence_failed") return "persistence_failed";
  if (result.status !== "applied") return "unconfirmed";
  return result.effective === result.requested ? "applied" : "clamped";
}

function failureStatus(
  failure: { error?: unknown },
  changes: ConfigurationChangeOutcome[],
  scope?: PickerConfigurationFeedbackScope,
): FeedbackStatus {
  const result = errorResult(failure.error);
  if (resultStatus(result, scope) === "persistence_failed") return "persistence_failed";
  const statuses = changes.map((change) => resultStatus(change.result, scope));
  if (statuses.includes("applied") || statuses.includes("clamped")) return "partial";
  if (result?.status === "rejected") return "rejected";
  if (failure.error instanceof ApiError && failure.error.status < 500) return "rejected";
  return "unconfirmed";
}

function changesStatus(
  changes: ConfigurationChangeOutcome[],
  failed: { error?: unknown } | undefined,
  scope?: PickerConfigurationFeedbackScope,
): FeedbackStatus {
  if (failed) return failureStatus(failed, changes, scope);
  const statuses = changes.map((change) => resultStatus(change.result, scope));
  if (statuses.length > 0 && statuses.every((status) => status === "saved_for_launch")) {
    return "saved_for_launch";
  }
  const hasSaved = statuses.includes("saved_for_launch");
  const hasLive = statuses.includes("applied") || statuses.includes("clamped");
  const hasUnconfirmed = statuses.includes("unconfirmed");
  if (hasSaved && (hasLive || hasUnconfirmed)) return "mixed";
  if (statuses.includes("persistence_failed")) return "persistence_failed";
  if (hasUnconfirmed) return "unconfirmed";
  return hasLive && statuses.includes("clamped") ? "clamped" : "applied";
}

function valueSummary(
  changes: ConfigurationChangeOutcome[],
  effective: boolean,
  statuses?: FeedbackStatus[],
  scope?: PickerConfigurationFeedbackScope,
): string {
  return changes
    .map((change) => {
      if (statuses && !statuses.includes(resultStatus(change.result, scope))) return undefined;
      const value = effective ? change.result?.effective : change.requested;
      return value === undefined ? undefined : `${change.label}: ${value}`;
    })
    .filter((value): value is string => !!value)
    .join(", ");
}

function feedbackMessageText(feedback: PickerFeedback, t: ReturnType<typeof useTranslation>["t"]) {
  const requested = valueSummary(feedback.changes, false);
  const effective = valueSummary(feedback.changes, true, ["applied", "clamped"], feedback.scope);
  const safeReason = feedback.reason || t("task:sessionConfigurationRejectedReason");
  switch (feedback.status) {
    case "applied":
      return t("task:sessionConfigurationApplied", { value: effective });
    case "clamped":
      return t("task:sessionConfigurationClamped", { requested, effective });
    case "saved_for_launch":
      return t("task:sessionConfigurationSaved", {
        value: valueSummary(feedback.changes, false, ["saved_for_launch"], feedback.scope),
      });
    case "unconfirmed":
      return t("task:sessionConfigurationUnconfirmed", {
        value: valueSummary(feedback.changes, false, ["unconfirmed"], feedback.scope) || requested,
      });
    case "rejected":
      return t("task:sessionConfigurationRejected", { value: requested, reason: safeReason });
    case "persistence_failed":
      return t("task:sessionConfigurationPersistenceFailed", {
        value:
          valueSummary(feedback.changes, true, ["persistence_failed"], feedback.scope) ||
          effective ||
          requested,
        reason: safeReason,
      });
    case "mixed":
      return t("task:sessionConfigurationMixed", {
        applied: valueSummary(feedback.changes, true, ["applied", "clamped"], feedback.scope),
        saved: valueSummary(feedback.changes, false, ["saved_for_launch"], feedback.scope),
        unconfirmed: valueSummary(feedback.changes, false, ["unconfirmed"], feedback.scope),
      });
    case "partial":
      return t("task:sessionConfigurationPartial", {
        applied: valueSummary(feedback.changes, true, ["applied", "clamped"], feedback.scope),
        failed: feedback.failed ? `${feedback.failed.label}: ${feedback.failed.value}` : "",
        reason: safeReason,
      });
  }
}

function feedbackPresentation(status: FeedbackStatus) {
  if (status === "applied" || status === "clamped") {
    return {
      passive: true,
      className: "border-emerald-500/30 text-emerald-700 dark:text-emerald-300",
      icon: <IconCheck className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />,
    };
  }
  if (status === "rejected" || status === "persistence_failed") {
    return {
      passive: false,
      className: "border-destructive/40 text-destructive",
      icon: <IconX className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />,
    };
  }
  if (status === "partial") {
    return {
      passive: false,
      className: "border-border text-foreground",
      icon: <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden />,
    };
  }
  return {
    passive: false,
    className: "border-border text-foreground",
    icon: <IconInfoCircle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />,
  };
}

function PickerFeedbackMessage({
  feedback,
  dismiss,
}: {
  feedback: PickerFeedback;
  dismiss: () => void;
}) {
  const { t } = useTranslation();
  const presentation = feedbackPresentation(feedback.status);
  const message = feedbackMessageText(feedback, t);
  const isPositive = ["applied", "clamped"].includes(feedback.status);
  const canDismiss = !isPositive;

  return (
    <div
      className={cn(
        "flex max-w-[min(26rem,90vw)] items-start gap-2 rounded-md border bg-popover px-2.5 py-2 text-xs shadow-md",
        presentation.passive && "pointer-events-none",
        presentation.className,
      )}
      role="status"
      aria-live="polite"
      data-testid="session-configuration-feedback"
      data-status={feedback.status}
    >
      {presentation.icon}
      <span className="min-w-0 flex-1 break-words">
        {message}
        {feedback.status === "unconfirmed" && feedback.reason && (
          <span className="mt-1 block text-muted-foreground">{feedback.reason}</span>
        )}
      </span>
      {feedback.retry && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 min-w-11 shrink-0 px-2 [@media(pointer:coarse)]:h-11"
          data-testid="session-configuration-feedback-retry"
          onClick={feedback.retry}
        >
          {t("task:sessionConfigurationRetry")}
        </Button>
      )}
      {canDismiss && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-7 w-7 min-w-11 shrink-0 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          aria-label={t("common:close")}
          data-testid="session-configuration-feedback-dismiss"
          onClick={dismiss}
        >
          <IconX className="h-3.5 w-3.5" aria-hidden />
        </Button>
      )}
    </div>
  );
}

function useFeedbackExpiry(
  feedback: PickerFeedback | undefined,
  setFeedback: Dispatch<SetStateAction<PickerFeedback | undefined>>,
  isPickerOpen: boolean,
  documentVisible: boolean,
) {
  const timerState = useRef<{
    id: number;
    remaining: number;
    startedAt?: number;
  }>({ id: 0, remaining: 1000 });

  useEffect(() => {
    if (
      !feedback ||
      !["applied", "clamped"].includes(feedback.status) ||
      isPickerOpen ||
      !documentVisible
    ) {
      return;
    }
    if (timerState.current.id !== feedback.id) {
      timerState.current = { id: feedback.id, remaining: 1000 };
    }
    const id = feedback.id;
    const startedAt = Date.now();
    timerState.current.startedAt = startedAt;
    const timeout = window.setTimeout(() => {
      timerState.current.remaining = 0;
      timerState.current.startedAt = undefined;
      setFeedback((current) => (current?.id === id ? undefined : current));
    }, timerState.current.remaining);
    return () => {
      window.clearTimeout(timeout);
      if (timerState.current.id === id && timerState.current.startedAt === startedAt) {
        timerState.current.remaining = Math.max(
          0,
          timerState.current.remaining - (Date.now() - startedAt),
        );
        timerState.current.startedAt = undefined;
      }
    };
  }, [documentVisible, feedback?.id, feedback?.status, isPickerOpen, setFeedback]);
}

function sameFeedbackScope(
  requested: PickerConfigurationFeedbackScope | undefined,
  current: PickerConfigurationFeedbackScope | undefined,
  executionIds: string[] = [],
): boolean {
  if (requested?.sessionId !== current?.sessionId) return false;
  if (requested?.executionId === current?.executionId) {
    return !current?.executionId || executionIds.every((id) => id === current.executionId);
  }
  return (
    !requested?.executionId &&
    !!current?.executionId &&
    executionIds.length > 0 &&
    executionIds.every((id) => id === current.executionId)
  );
}

function resultExecutionIds(changes: ConfigurationChangeOutcome[], error?: unknown): string[] {
  const ids = changes
    .map((change) => change.result?.execution_id)
    .filter((id): id is string => !!id);
  const errorId = errorResult(error)?.execution_id;
  if (errorId) ids.push(errorId);
  return ids;
}

function createFeedbackController({
  latestId,
  requestScopes,
  scopeRef,
  setFeedback,
}: {
  latestId: RefObject<number>;
  requestScopes: RefObject<Map<number, PickerConfigurationFeedbackScope | undefined>>;
  scopeRef: RefObject<PickerConfigurationFeedbackScope | undefined>;
  setFeedback: Dispatch<SetStateAction<PickerFeedback | undefined>>;
}): PickerConfigurationFeedbackController {
  const isCurrentRequest = (
    id: number,
    changes: ConfigurationChangeOutcome[] = [],
    error?: unknown,
  ) =>
    id === latestId.current &&
    sameFeedbackScope(
      requestScopes.current.get(id),
      scopeRef.current,
      resultExecutionIds(changes, error),
    );
  const begin = () => {
    const id = latestId.current + 1;
    latestId.current = id;
    requestScopes.current.set(id, scopeRef.current ? { ...scopeRef.current } : undefined);
    setFeedback(undefined);
    return id;
  };
  const showChanges: PickerConfigurationFeedbackController["showChanges"] = (
    id,
    changes,
    failure,
    retry,
  ) => {
    if (!isCurrentRequest(id, changes, failure?.error)) return;
    setFeedback({
      id,
      status: changesStatus(changes, failure, scopeRef.current),
      changes,
      failed: failure ? { label: failure.label, value: failure.value } : undefined,
      reason: failure ? sanitizeSessionErrorDetails(failure.error) : undefined,
      retry,
      scope: scopeRef.current,
    });
  };
  const showResult: PickerConfigurationFeedbackController["showResult"] = (
    id,
    label,
    requested,
    result,
  ) => {
    const changes = [{ label, requested, result }];
    if (!isCurrentRequest(id, changes)) return;
    setFeedback({
      id,
      status: resultStatus(result, scopeRef.current),
      changes,
      scope: scopeRef.current,
    });
  };
  const showError: PickerConfigurationFeedbackController["showError"] = (
    id,
    label,
    requested,
    error,
  ) => {
    const result = errorResult(error);
    if (result) {
      if (!isCurrentRequest(id, [{ label, requested, result }], error)) return;
      showResult(id, label, requested, result);
      setFeedback((current) =>
        current?.id === id ? { ...current, reason: sanitizeSessionErrorDetails(error) } : current,
      );
      return;
    }
    if (!isCurrentRequest(id, [], error)) return;
    setFeedback({
      id,
      status: error instanceof ApiError && error.status < 500 ? "rejected" : "unconfirmed",
      changes: [{ label, requested }],
      reason: sanitizeSessionErrorDetails(error),
      scope: scopeRef.current,
    });
  };
  return { begin, showResult, showError, showChanges };
}

export function usePickerConfigurationFeedback(
  isPickerOpen: boolean,
  scope?: PickerConfigurationFeedbackScope,
) {
  const [feedback, setFeedback] = useState<PickerFeedback>();
  const [documentVisible, setDocumentVisible] = useState(true);
  const latestId = useRef(0);
  const scopeRef = useRef(scope);
  scopeRef.current = scope;
  const requestScopes = useRef(new Map<number, PickerConfigurationFeedbackScope | undefined>());
  useEffect(() => {
    const updateVisibility = () => setDocumentVisible(document.visibilityState === "visible");
    updateVisibility();
    document.addEventListener("visibilitychange", updateVisibility);
    return () => document.removeEventListener("visibilitychange", updateVisibility);
  }, []);
  useFeedbackExpiry(feedback, setFeedback, isPickerOpen, documentVisible);

  const controller = useMemo(
    () => createFeedbackController({ latestId, requestScopes, scopeRef, setFeedback }),
    [],
  );
  const dismiss = useCallback(() => setFeedback(undefined), []);
  const visibleFeedback =
    feedback &&
    sameFeedbackScope(feedback.scope, scopeRef.current, resultExecutionIds(feedback.changes))
      ? feedback
      : undefined;

  return {
    controller,
    feedback: visibleFeedback,
    dismiss,
    passive: !!visibleFeedback && ["applied", "clamped"].includes(visibleFeedback.status),
    message: visibleFeedback ? (
      <PickerFeedbackMessage feedback={visibleFeedback} dismiss={dismiss} />
    ) : null,
  };
}
