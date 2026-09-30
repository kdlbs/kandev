"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { SessionErrorDetails } from "@/components/task/session-error-details";
import {
  hasSessionRecoveryResolutionAfter,
  isSelectionFailureRecoveryMetadata,
} from "@/hooks/processed-message-filtering";
import {
  lastAgentErrorStamp,
  normalizeAgentErrorCauses,
  readLastAgentError,
  readLastAgentErrorIncludingDismissed,
} from "@/lib/session-last-agent-error";
import { buildRecoveryCardModel } from "@/components/task/chat/session-bootstrap-recovery-model";
import { legacyRecoveryMessageMatchesError } from "@/lib/session-recovery-presentation";
import type { Message } from "@/lib/types/http";
import type { ActionMeta } from "./action-message-details";
import { readableFailureSummary } from "./action-message-utils";

export function isRecoveryMessageDismissed(
  comment: Message,
  metadata: ActionMeta | undefined,
  sessionMetadata: Record<string, unknown> | null | undefined,
): boolean {
  const dismissedError = readLastAgentErrorIncludingDismissed(sessionMetadata);
  if (!dismissedError?.dismissedAt) return false;
  const stamp = metadata?.recovery_stamp ?? metadata?.error_stamp;
  if (stamp) return lastAgentErrorStamp(dismissedError) === stamp;
  return legacyRecoveryMessageMatchesError(comment.content, comment.created_at, dismissedError);
}

export function resolveRecoveryHistoryState(
  comment: Message,
  metadata: ActionMeta | undefined,
  ownerMessageId: string | undefined,
  sessionMetadata: Record<string, unknown> | null | undefined,
): { activeOwner: boolean } | null {
  if (!metadata?.recovery_actions) return null;
  const currentError = readLastAgentError(sessionMetadata);
  const messageStamp = metadata.recovery_stamp ?? metadata.error_stamp;
  const currentStamp = currentError ? lastAgentErrorStamp(currentError) : undefined;
  const activeOwner = ownerMessageId === comment.id;
  const isHistorical = Boolean(messageStamp && currentStamp && messageStamp !== currentStamp);
  if (
    activeOwner ||
    isRecoveryMessageDismissed(comment, metadata, sessionMetadata) ||
    isHistorical
  ) {
    return { activeOwner };
  }
  return null;
}

export function RecoveryHistory({
  comment,
  metadata,
  activeOwner = false,
}: {
  comment: Message;
  metadata?: ActionMeta;
  activeOwner?: boolean;
}) {
  const { t } = useTranslation();
  const sessionMetadata = useAppStore((state) =>
    comment.session_id ? state.taskSessions.items[comment.session_id]?.metadata : undefined,
  );
  const messages = useAppStore((state) =>
    comment.session_id ? state.messages.bySession[comment.session_id] : undefined,
  );
  const status = recoveryHistoryStatus(comment, metadata, sessionMetadata, messages);
  const typed = historicalRecoveryPresentation(comment, metadata, t);
  return (
    <RecoveryHistoryView
      comment={comment}
      metadata={metadata}
      activeOwner={activeOwner}
      status={status}
      typed={typed}
    />
  );
}

function RecoveryHistoryView({
  comment,
  metadata,
  activeOwner,
  status,
  typed,
}: {
  comment: Message;
  metadata?: ActionMeta;
  activeOwner: boolean;
  status: "resolved" | "dismissed" | null;
  typed: ReturnType<typeof historicalRecoveryPresentation>;
}) {
  const { t } = useTranslation();
  const summary =
    typed?.summary ?? readableFailureSummary(comment.content) ?? t("task:anErrorOccurred");
  const legacyDetails = typed ? "" : (metadata?.error_output ?? comment.content);
  const showDetails = Boolean(
    typed?.detailFields.length || (legacyDetails && legacyDetails !== summary),
  );
  return (
    <div
      className="min-w-0 py-2 text-xs text-muted-foreground"
      data-testid="session-recovery-history"
    >
      {status && <RecoveryStatusBadge status={status} />}
      {activeOwner ? (
        <p className="wrap-anywhere">{t("task:sessionRecoveryDetailsAbove")}</p>
      ) : (
        <>
          <p className="wrap-anywhere">{summary}</p>
          {showDetails && (
            <SessionErrorDetails
              testId="session-recovery-history-details"
              structuredFields={typed?.detailFields}
            >
              {legacyDetails}
            </SessionErrorDetails>
          )}
        </>
      )}
    </div>
  );
}

function RecoveryStatusBadge({ status }: { status: "resolved" | "dismissed" }) {
  const { t } = useTranslation();
  const testId = status === "resolved" ? "session-recovery-resolved" : "session-recovery-dismissed";
  const label =
    status === "resolved" ? t("task:sessionRecoveryResolved") : t("task:sessionRecoveryDismissed");
  return (
    <span
      className="mb-1 inline-flex rounded border px-1.5 py-0.5 text-[10px]"
      data-testid={testId}
    >
      {label}
    </span>
  );
}

function recoveryHistoryStatus(
  comment: Message,
  metadata: ActionMeta | undefined,
  sessionMetadata: Record<string, unknown> | null | undefined,
  messages: readonly Message[] | undefined,
): "resolved" | "dismissed" | null {
  const errorStamp = metadata?.recovery_stamp ?? metadata?.error_stamp;
  const lastError = readLastAgentErrorIncludingDismissed(sessionMetadata);
  const requireExactStamp =
    isSelectionFailureRecoveryMetadata(metadata) || isSelectionFailureRecoveryMetadata(lastError);
  if (
    hasSessionRecoveryResolutionAfter(
      sessionMetadata,
      comment.created_at,
      errorStamp,
      messages,
      requireExactStamp,
    )
  ) {
    return "resolved";
  }
  return isRecoveryMessageDismissed(comment, metadata, sessionMetadata) ? "dismissed" : null;
}

function historicalRecoveryPresentation(
  comment: Message,
  metadata: ActionMeta | undefined,
  translate: ReturnType<typeof useTranslation>["t"],
) {
  const causes = normalizeAgentErrorCauses(metadata?.causes);
  if (!hasHistoricalRecoveryEvidence(causes, metadata)) return null;
  return buildRecoveryCardModel({
    error: {
      session_id: comment.session_id ?? undefined,
      stamp: String(metadata?.recovery_stamp ?? metadata?.error_stamp ?? ""),
      occurred_at: comment.created_at,
      preview: comment.content,
      phase: metadata?.phase,
      attempt_id: metadata?.attempt_id,
      execution_id: metadata?.execution_id,
      causes,
    },
    manualFailure: null,
    manualError: null,
    recoveryNotice: null,
    translate,
  });
}

function hasHistoricalRecoveryEvidence(
  causes: ReturnType<typeof normalizeAgentErrorCauses>,
  metadata: ActionMeta | undefined,
): boolean {
  return (
    causes.length > 0 || Boolean(metadata?.phase || metadata?.attempt_id || metadata?.execution_id)
  );
}
