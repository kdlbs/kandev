import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useAppStore } from "@/components/state-provider";
import { readAgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";
import { sessionDeliveryRecoveryMessage } from "@/lib/services/session-recovery-service";
import {
  useInterruptedRecoveryBatch,
  type InterruptedRecoveryCandidate,
} from "@/hooks/domains/session/use-interrupted-recovery-batch";

export function InterruptedSessionsRecovery() {
  const sessions = useAppStore((state) => state.taskSessions.items);
  const tasks = useAppStore((state) => state.kanban.tasks);
  const candidates: InterruptedRecoveryCandidate[] = Object.values(sessions).flatMap((session) => {
    const recovery = readAgentDeliveryRecovery(session.metadata);
    if (!recovery || !["uncertain", "reconnecting", "continued"].includes(recovery.phase))
      return [];
    return [
      {
        taskId: session.task_id,
        sessionId: session.id,
        label:
          tasks.find((task) => task.id === session.task_id)?.title ??
          session.name ??
          session.id.slice(0, 8),
        observed: {
          task_id: session.task_id,
          session_id: session.id,
          outcome: "uncertain" as const,
          recovery_revision: recovery.revision,
          recovery_identity: {
            submission_id: recovery.submissionId,
            stream_id: recovery.streamId,
            incarnation_id: recovery.incarnationId,
            harness_generation: recovery.harnessGeneration,
            prompt_generation: recovery.promptGeneration,
          },
        },
      },
    ];
  });
  return <InterruptedSessionsRecoveryContent candidates={candidates} />;
}

function InterruptedSessionsRecoveryContent({
  candidates,
}: {
  candidates: InterruptedRecoveryCandidate[];
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [instruction, setInstruction] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const batch = useInterruptedRecoveryBatch(candidates);
  if (!candidates.length) return null;
  return (
    <div
      className="w-full min-w-0 shrink-0 px-3 pt-3 sm:px-4"
      data-testid="interrupted-sessions-notice"
    >
      <div className="min-w-0 rounded-md border bg-background p-3">
        <Button
          type="button"
          variant="outline"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className={controlSizingClassName(
            "standard",
            "w-full cursor-pointer whitespace-normal md:w-auto",
          )}
          data-testid="interrupted-sessions-open"
        >
          {t("task:interruptedRecoveryTitle")}
        </Button>
        {open && (
          <div className="mt-3 min-w-0 space-y-2" data-testid="interrupted-sessions-controls">
            <p className="text-sm">{t("task:interruptedRecoverySelect")}</p>
            <InterruptedSessionChoices candidates={candidates} batch={batch} />
            <label className="block text-sm">
              {t("task:interruptedRecoveryInstruction")}
              <Textarea
                value={instruction}
                onChange={(event) => setInstruction(event.target.value)}
                disabled={batch.busy}
                className="mt-1 min-h-20"
                data-testid="interrupted-batch-instruction"
              />
            </label>
            <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={acknowledged}
                onChange={(event) => setAcknowledged(event.target.checked)}
                disabled={batch.busy}
                className="size-4 shrink-0"
                data-testid="interrupted-batch-acknowledge"
              />
              {t("task:interruptedRecoveryAcknowledgment")}
            </label>
            <Button
              type="button"
              disabled={
                batch.busy || !batch.selected.length || !acknowledged || !instruction.trim()
              }
              onClick={() => void batch.run(instruction, acknowledged)}
              className={controlSizingClassName(
                "standard",
                "w-full cursor-pointer whitespace-normal md:w-auto",
              )}
              data-testid="interrupted-batch-resume"
            >
              {t("task:interruptedRecoveryBatchResume")}
            </Button>
            {batch.busy && (
              <p role="status" className="text-sm">
                {t("task:interruptedRecoveryProgress", {
                  current: Math.min(batch.completed + 1, batch.selected.length),
                  total: batch.selected.length,
                })}
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function InterruptedSessionChoices({
  candidates,
  batch,
}: {
  candidates: InterruptedRecoveryCandidate[];
  batch: ReturnType<typeof useInterruptedRecoveryBatch>;
}) {
  const { t } = useTranslation();
  return candidates.map((item) => (
    <div key={item.sessionId} className="min-w-0">
      <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={batch.selected.includes(item.sessionId)}
          disabled={batch.busy}
          onChange={() => batch.toggle(item.sessionId)}
          className="size-4 shrink-0"
          data-testid={`interrupted-select-${item.sessionId}`}
        />
        <span className="break-words">{item.label}</span>
      </label>
      {batch.results[item.sessionId] && (
        <p
          role="status"
          className="break-words text-xs"
          data-testid={`interrupted-result-${item.sessionId}`}
        >
          {sessionDeliveryRecoveryMessage(batch.results[item.sessionId], t)}
        </p>
      )}
      {batch.failures.includes(item.sessionId) && (
        <p role="status" className="break-words text-xs">
          {t("task:interruptedRecoveryFailed")}
        </p>
      )}
    </div>
  ));
}
