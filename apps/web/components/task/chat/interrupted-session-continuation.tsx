import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { SessionDeliveryRecoveryResponse } from "@/lib/services/session-recovery-service";
import { sessionDeliveryRecoveryMessage } from "@/lib/services/session-recovery-service";
import {
  continueInterruptedSession,
  interruptedRecoveryKey,
  readInterruptedCheckpoint,
} from "@/lib/services/interrupted-session-recovery";

export function InterruptedSessionContinuation({
  observed,
}: {
  observed: SessionDeliveryRecoveryResponse;
}) {
  return (
    <InterruptedSessionContinuationForm
      key={`${interruptedRecoveryKey(observed)}:${observed.recovery_revision}`}
      observed={observed}
    />
  );
}

function InterruptedSessionContinuationForm({
  observed,
}: {
  observed: SessionDeliveryRecoveryResponse;
}) {
  const { t } = useTranslation();
  const saved = readInterruptedCheckpoint(observed.task_id, observed.session_id, observed);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [instruction, setInstruction] = useState(saved?.request.instruction ?? "");
  const [acknowledged, setAcknowledged] = useState(Boolean(saved));
  const [busy, setBusy] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [notice, setNotice] = useState(
    saved?.result ? sessionDeliveryRecoveryMessage(saved.result, t) : "",
  );
  const submit = async () => {
    setBusy(true);
    setNotice("");
    try {
      const completed = await continueInterruptedSession({
        taskId: observed.task_id,
        sessionId: observed.session_id,
        observed,
        instruction,
        acknowledged,
        failureMessage: t("task:interruptedRecoveryFailed"),
      });
      if (mounted.current && completed.result)
        setNotice(sessionDeliveryRecoveryMessage(completed.result, t));
    } catch {
      if (mounted.current) setNotice(t("task:interruptedRecoveryFailed"));
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  if (hidden) return null;
  return (
    <div className="mt-3 min-w-0 space-y-2" data-testid="interrupted-session-continuation">
      <label className="block text-sm">
        {t("task:interruptedRecoveryInstruction")}
        <Textarea
          value={instruction}
          onChange={(event) => setInstruction(event.target.value)}
          disabled={busy || Boolean(saved)}
          data-testid="interrupted-recovery-instruction"
          className="mt-1 min-h-20 w-full"
        />
      </label>
      <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
          disabled={busy}
          data-testid="interrupted-recovery-acknowledge"
          className="size-4 shrink-0"
        />
        {t("task:interruptedRecoveryAcknowledgment")}
      </label>
      <div className="flex min-w-0 flex-col gap-2 md:flex-row">
        <Button
          type="button"
          disabled={busy || !acknowledged || !instruction.trim()}
          onClick={() => void submit()}
          data-testid="interrupted-recovery-resume"
          className={controlSizingClassName(
            "standard",
            "w-full cursor-pointer whitespace-normal md:w-auto",
          )}
        >
          {t("task:interruptedRecoveryResume")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          onClick={() => setHidden(true)}
          className={controlSizingClassName("standard", "w-full cursor-pointer md:w-auto")}
        >
          {t("task:interruptedRecoveryCancel")}
        </Button>
      </div>
      {(busy || notice) && (
        <p role="status" className="break-words text-sm" data-testid="interrupted-recovery-result">
          {busy ? t("task:resuming") : notice}
        </p>
      )}
    </div>
  );
}
