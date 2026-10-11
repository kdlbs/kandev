import type { TFunction } from "i18next";
import type { SessionRecoveryBlockProjection } from "@/lib/types/http";
import type { AgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";
import { sessionDeliveryRecoveryReasonMessage } from "@/lib/services/session-recovery-service";

export function deliveryRecoveryRequestIdentity(
  recovery: AgentDeliveryRecovery | null,
  block: SessionRecoveryBlockProjection | undefined,
): string | null {
  if (recovery) {
    return JSON.stringify([
      recovery.revision,
      recovery.submissionId,
      recovery.streamId,
      recovery.incarnationId,
      recovery.harnessGeneration,
      recovery.promptGeneration,
    ]);
  }
  if (!block) return null;
  return `block:${block.id}:${block.incarnation_id}:${block.expected_generation}:${block.updated_at}`;
}

export function persistedDeliveryRecoveryNotice(
  recovery: AgentDeliveryRecovery | null,
  block: SessionRecoveryBlockProjection | undefined,
  t: TFunction,
): string | null {
  if (recovery?.reconstruction) {
    return recovery.reconstruction.processIdentityKnown
      ? null
      : sessionDeliveryRecoveryReasonMessage("recovery_identity_incomplete", t);
  }
  if (block) return sessionDeliveryRecoveryReasonMessage(block.reason, t);
  return null;
}

export function isDeliveryRecoveryResultCurrent(
  localResultIsCurrent: boolean,
  result: { allowed_actions?: string[]; recovery_revision?: number } | null,
  recoveryRevision: number | undefined,
): boolean {
  if (!localResultIsCurrent) return false;
  if (!result?.allowed_actions?.length || recoveryRevision === undefined) return true;
  return result.recovery_revision === recoveryRevision;
}
