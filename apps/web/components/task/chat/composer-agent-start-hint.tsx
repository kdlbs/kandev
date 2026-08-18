"use client";

import { useTranslation } from "react-i18next";

/**
 * The composer's "a message will auto-start the agent" hint for
 * recovered-idle (resume-skipped) sessions, shown while the agent is
 * stopped AND the composer can actually accept a send. Informational only:
 * no click action.
 *
 * The extra gates mirror the composer's own send-availability signals: a
 * session in recovery (needsRecovery — WAITING_FOR_INPUT with an error
 * message) disables the editor, an unavailable executor environment
 * replaces it with the stopped-session banner, and a pending clarification
 * routes every submission to the message queue (which only persists —
 * it does not resume the stopped agent). In all three cases no message can
 * be delivered to the agent, so promising that sending will start it would
 * be a dead end.
 */
export function ComposerAgentStartHint({
  show,
  needsRecovery = false,
  executorUnavailable = false,
  hasPendingClarification = false,
}: {
  show: boolean;
  needsRecovery?: boolean;
  executorUnavailable?: boolean;
  hasPendingClarification?: boolean;
}) {
  const { t } = useTranslation();
  if (!show || needsRecovery || executorUnavailable || hasPendingClarification) return null;
  return (
    <p data-testid="composer-agent-start-hint" className="px-1 pb-1 text-xs text-muted-foreground">
      {t("task:composerStartAgentHint")}
    </p>
  );
}
