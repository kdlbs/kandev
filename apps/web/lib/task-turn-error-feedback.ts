import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";

/**
 * Nonfatal feedback for an accepted prompt whose client-observed turn-error
 * acknowledgement could not be persisted. The prompt stays accepted and the
 * badge stays visible; callers must never resend to repair the metadata.
 */
export function notifyTurnErrorAcknowledgementFailure(failed: boolean | undefined): void {
  if (!failed) return;
  toast.error(t("task:turnErrorAcknowledgementFailed"));
}
