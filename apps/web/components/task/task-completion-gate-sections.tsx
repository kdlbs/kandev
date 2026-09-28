"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";
import type {
  TaskCompletionGateHistory,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";

export function TaskCompletionGateBlockers({
  snapshot,
}: {
  snapshot: TaskCompletionGateSnapshot | null;
}) {
  const { t } = useTranslation();
  if (!snapshot?.blocked) return null;
  return (
    <div
      className="rounded-md border border-destructive/40 bg-destructive/5 p-3"
      data-testid="task-completion-gate-blockers"
    >
      <p className="text-sm font-medium">{t("task:completionGateBlocked")}</p>
      <ul className="mt-2 flex flex-col gap-2">
        {(snapshot.blockers ?? []).map((blocker) => {
          const criterion = snapshot.criteria.find((item) => item.id === blocker.criterion_id);
          return (
            <li
              key={blocker.criterion_id}
              className="text-sm"
              data-testid="task-completion-gate-blocker"
            >
              <span className="font-medium">{criterion?.description ?? blocker.criterion_id}</span>
              <span className="ml-2 text-muted-foreground">
                {t(`task:completionGateBlocker_${blocker.reason}`, {
                  defaultValue: blocker.reason,
                })}
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

type OverrideSectionProps = {
  visible: boolean;
  completionSteps: WorkflowStepperStep[];
  selectedStep: string;
  reason: string;
  saving: boolean;
  onStepChange: (stepId: string) => void;
  onReasonChange: (reason: string) => void;
  onSubmit: () => void;
};

export function TaskCompletionOverrideSection(props: OverrideSectionProps) {
  const { t } = useTranslation();
  if (!props.visible) return null;
  return (
    <section
      className="flex flex-col gap-3 rounded-md border border-border p-3"
      aria-labelledby="task-completion-gate-override-heading"
    >
      <h3 id="task-completion-gate-override-heading" className="text-sm font-medium">
        {t("task:completionGateOverrideTitle")}
      </h3>
      <p className="text-sm text-muted-foreground">{t("task:completionGateOverrideDescription")}</p>
      {props.completionSteps.length > 0 ? (
        <OverrideForm {...props} />
      ) : (
        <p className="text-sm text-muted-foreground">{t("task:completionGateNoCompletionStep")}</p>
      )}
    </section>
  );
}

function OverrideForm(props: OverrideSectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-override-step">
        {t("task:completionGateOverrideStep")}
        <select
          id="task-completion-gate-override-step"
          className="min-h-11 rounded-md border border-input bg-background px-3"
          value={props.selectedStep}
          onChange={(event) => props.onStepChange(event.currentTarget.value)}
          data-testid="task-completion-gate-override-step"
        >
          <option value="">{t("task:completionGateSelectStep")}</option>
          {props.completionSteps.map((step) => (
            <option key={step.id} value={step.id}>
              {step.name}
            </option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-override-reason">
        {t("task:completionGateOverrideReason")}
        <Textarea
          id="task-completion-gate-override-reason"
          className="min-h-20 resize-y"
          value={props.reason}
          onChange={(event) => props.onReasonChange(event.currentTarget.value)}
          data-testid="task-completion-gate-override-reason"
        />
      </label>
      <Button
        type="button"
        variant="destructive"
        className="min-h-11"
        disabled={!props.selectedStep || !props.reason.trim() || props.saving}
        onClick={props.onSubmit}
        data-testid="task-completion-gate-override"
      >
        {props.saving ? t("task:completionGateSaving") : t("task:completionGateOverrideAction")}
      </Button>
    </>
  );
}

export function TaskCompletionGateHistorySection({
  history,
}: {
  history: TaskCompletionGateHistory[];
}) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-2" aria-labelledby="task-completion-gate-history-heading">
      <h3 id="task-completion-gate-history-heading" className="text-sm font-medium">
        {t("task:completionGateHistory")}
      </h3>
      <ol className="flex flex-col gap-2" data-testid="task-completion-gate-history">
        {history.map((item) => (
          <li
            key={item.id}
            className="rounded-md border border-border p-3 text-sm"
            data-testid="task-completion-gate-history-item"
          >
            <div className="font-medium">
              {t(`task:completionGateAction_${item.action}`, { defaultValue: item.action })}
            </div>
            <div className="break-all text-xs text-muted-foreground">{item.actor_id}</div>
            {item.reason && <div className="pt-1 text-xs">{item.reason}</div>}
          </li>
        ))}
        {history.length === 0 && (
          <li className="text-sm text-muted-foreground">{t("task:completionGateNoHistory")}</li>
        )}
      </ol>
    </section>
  );
}
