import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Combobox } from "@/components/combobox";
import type { ComboboxOption } from "@/components/combobox";
import { WorkflowMovePreviewFooter } from "@/components/task/workflow-move-preview-footer";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useChangeWorkflow } from "@/hooks/domains/kanban/use-change-workflow";
import type { WorkflowStepDTO } from "@/lib/types/http";

export type ChangeWorkflowState = ReturnType<typeof useChangeWorkflow>;
type TouchProps = { isTouchSurface: boolean };

function RetryButton({
  onClick,
  label,
  testId,
}: {
  onClick: () => void;
  label: string;
  testId?: string;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="min-h-9 gap-2"
      onClick={onClick}
      data-testid={testId}
    >
      <IconRefresh className="size-3.5" aria-hidden="true" />
      {label}
    </Button>
  );
}

export function ChangeWorkflowCurrentTask({ state }: { state: ChangeWorkflowState }) {
  const { t } = useTranslation();
  return (
    <>
      {state.taskStatus === "loading" && (
        <div
          role="status"
          className="flex min-h-9 items-center gap-2 text-sm text-muted-foreground"
        >
          <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />
          {t("task:changeWorkflowLoadingTask")}
        </div>
      )}
      {state.taskStatus === "error" && (
        <div
          role="alert"
          className="grid justify-items-start gap-2 rounded-md border border-destructive/40 p-3 text-sm"
        >
          <span>{t("task:changeWorkflowLoadTaskFailed")}</span>
          <RetryButton onClick={state.retryTask} label={t("task:changeWorkflowRetry")} />
        </div>
      )}
      {state.task && (
        <section
          aria-label={t("task:changeWorkflowCurrentLabel")}
          className="grid gap-1 rounded-md bg-muted/50 p-3"
        >
          <span className="text-xs font-medium text-muted-foreground">
            {t("task:changeWorkflowCurrentLabel")}
          </span>
          <span className="break-words text-sm font-medium">
            {t("task:changeWorkflowCurrentAssignment", {
              workflow: state.sourceWorkflowName ?? state.task.workflow_id,
              step: state.sourceStepName ?? state.task.workflow_step_id,
            })}
          </span>
          <span className="break-words text-xs text-muted-foreground">{state.task.title}</span>
        </section>
      )}
      {state.sourceChanged && (
        <div
          role="status"
          className="rounded-md border border-amber-500/50 bg-amber-500/10 p-3 text-sm"
        >
          {t("task:changeWorkflowSourceChanged")}
        </div>
      )}
    </>
  );
}

function stepOptions(steps: readonly WorkflowStepDTO[]): ComboboxOption[] {
  return [...steps]
    .sort((left, right) => left.position - right.position || left.id.localeCompare(right.id))
    .map((step) => ({
      value: step.id,
      label: step.name,
      renderLabel: () => (
        <span className="flex min-w-0 items-center gap-2">
          <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: step.color }} />
          <span className="truncate">{step.name}</span>
        </span>
      ),
    }));
}

function WorkflowPickerSection({
  state,
  isTouchSurface,
  workflowSelectId,
}: {
  state: ChangeWorkflowState;
  isTouchSurface: boolean;
  workflowSelectId: string;
}) {
  const { t } = useTranslation();
  const rowClass = cn("grid gap-1.5", isTouchSurface && "gap-2");
  const controlClass = isTouchSurface ? "h-11 w-full" : "h-7 w-full";
  const workflows: ComboboxOption[] = state.destinations.map((workflow) => ({
    value: workflow.id,
    label: workflow.name,
  }));
  return (
    <>
      {state.workflowsStatus === "loading" && (
        <LoadingStatus label={t("task:changeWorkflowLoadingWorkflows")} />
      )}
      {state.workflowsStatus === "error" && (
        <LoadError
          label={t("task:changeWorkflowLoadWorkflowsFailed")}
          onRetry={state.retryWorkflows}
        />
      )}
      {state.workflowsStatus === "success" && state.destinations.length === 0 && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("task:changeWorkflowNoDestinations")}
        </p>
      )}
      {state.workflowsStatus === "success" && state.destinations.length > 0 && (
        <div className={rowClass}>
          <span className="text-sm font-medium">{t("task:changeWorkflowDestinationLabel")}</span>
          <Combobox
            options={workflows}
            value={state.selectedWorkflowId}
            onValueChange={state.changeWorkflow}
            ariaLabel={t("task:changeWorkflowDestinationLabel")}
            triggerId={workflowSelectId}
            placeholder={t("task:changeWorkflowDestinationPlaceholder")}
            searchPlaceholder={t("task:changeWorkflowSearchWorkflows")}
            emptyMessage={t("task:changeWorkflowNoDestinationFound")}
            dropdownLabel={t("task:changeWorkflowDestinationLabel")}
            disabled={state.workflowsStatus !== "success" || state.isSubmitting}
            touchTarget={isTouchSurface}
            triggerClassName={controlClass}
            testId="change-workflow-destination"
            popoverPortal
          />
        </div>
      )}
    </>
  );
}

function StepPickerSection({
  state,
  isTouchSurface,
  stepSelectId,
}: {
  state: ChangeWorkflowState;
  isTouchSurface: boolean;
  stepSelectId: string;
}) {
  const { t } = useTranslation();
  const steps = state.snapshot?.steps ?? [];
  const rowClass = cn("grid gap-1.5", isTouchSurface && "gap-2");
  const controlClass = isTouchSurface ? "h-11 w-full" : "h-7 w-full";
  return (
    <>
      {state.selectedWorkflowId && state.snapshotStatus === "loading" && (
        <LoadingStatus label={t("task:changeWorkflowLoadingSteps")} />
      )}
      {state.selectedWorkflowId && state.snapshotStatus === "error" && (
        <LoadError label={t("task:changeWorkflowLoadStepsFailed")} onRetry={state.retrySnapshot} />
      )}
      {state.snapshotStatus === "success" && steps.length === 0 && (
        <p
          role="status"
          data-testid="change-workflow-no-steps"
          className="text-sm text-muted-foreground"
        >
          {t("task:changeWorkflowNoSteps")}
        </p>
      )}
      {state.snapshotStatus === "success" && steps.length > 0 && (
        <div className={rowClass}>
          <span className="text-sm font-medium">{t("task:changeWorkflowStepLabel")}</span>
          <Combobox
            options={stepOptions(steps)}
            value={state.selectedStepId}
            onValueChange={state.setSelectedStepId}
            ariaLabel={t("task:changeWorkflowStepLabel")}
            triggerId={stepSelectId}
            placeholder={t("task:changeWorkflowStepPlaceholder")}
            searchPlaceholder={t("task:changeWorkflowSearchSteps")}
            emptyMessage={t("task:changeWorkflowNoStepFound")}
            dropdownLabel={t("task:changeWorkflowStepLabel")}
            disabled={state.snapshotStatus !== "success" || state.isSubmitting}
            touchTarget={isTouchSurface}
            triggerClassName={controlClass}
            testId="change-workflow-step"
            popoverPortal
          />
        </div>
      )}
    </>
  );
}

export function ChangeWorkflowSelectors({
  state,
  isTouchSurface,
  workflowSelectId,
  stepSelectId,
}: {
  state: ChangeWorkflowState;
  isTouchSurface: boolean;
  workflowSelectId: string;
  stepSelectId: string;
}) {
  return (
    <>
      <WorkflowPickerSection
        state={state}
        isTouchSurface={isTouchSurface}
        workflowSelectId={workflowSelectId}
      />
      <StepPickerSection
        state={state}
        isTouchSurface={isTouchSurface}
        stepSelectId={stepSelectId}
      />
    </>
  );
}

function LoadingStatus({ label }: { label: string }) {
  return (
    <div role="status" className="flex min-h-9 items-center gap-2 text-sm text-muted-foreground">
      <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />
      {label}
    </div>
  );
}

function LoadError({ label, onRetry }: { label: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      role="alert"
      className="grid justify-items-start gap-2 rounded-md border border-destructive/40 p-3 text-sm"
    >
      <span>{label}</span>
      <RetryButton onClick={onRetry} label={t("task:changeWorkflowRetry")} />
    </div>
  );
}

type ConversationRelationship =
  | { id: string; stepName: string; kind: "initial" }
  | { id: string; stepName: string; kind: "step"; targetName: string };

function conversationRelationships(steps: readonly WorkflowStepDTO[]): ConversationRelationship[] {
  return steps.flatMap<ConversationRelationship>((step) => {
    const target = step.session_target;
    if (target?.kind === "initial") return [{ id: step.id, stepName: step.name, kind: "initial" }];
    if (target?.kind !== "step") return [];
    const targetStep = steps.find((candidate) => candidate.id === target.step_id);
    return [
      {
        id: step.id,
        stepName: step.name,
        kind: "step",
        targetName: targetStep?.name ?? "",
      },
    ];
  });
}

function WorkflowAgentRow({
  state,
  row,
  isTouchSurface,
}: {
  state: ChangeWorkflowState;
  row: ChangeWorkflowState["rows"][number];
  isTouchSurface: boolean;
}) {
  const { t } = useTranslation();
  const invalid = !row.replacementAvailable || state.sourceProfileErrorId === row.sourceProfileId;
  const options: ComboboxOption[] = state.profileOptions.map((option) => ({
    ...option,
    disabled: false,
  }));
  if (!options.some((option) => option.value === row.replacementProfileId)) {
    options.unshift({
      value: row.replacementProfileId,
      label: row.replacementLabel ?? row.sourceLabel,
      disabled: true,
      disabledReason: t("task:changeWorkflowProfileUnavailable"),
    });
  }
  return (
    <div
      className={cn("grid gap-2 rounded-md border p-3", invalid && "border-destructive/50")}
      data-testid={`change-workflow-profile-${row.sourceProfileId}`}
    >
      <div className="grid gap-0.5">
        <span className="text-sm font-medium">{row.sourceLabel}</span>
        <span className="text-xs text-muted-foreground">
          {t("task:changeWorkflowProfileSteps", { steps: row.stepNames.join(", ") })}
        </span>
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <Combobox
          options={options}
          value={row.replacementProfileId}
          onValueChange={(value) => state.setOverride(row.sourceProfileId, value)}
          ariaLabel={t("task:changeWorkflowReplacementLabel", { profile: row.sourceLabel })}
          placeholder={t("task:changeWorkflowProfilePlaceholder")}
          searchPlaceholder={t("task:searchAgents")}
          emptyMessage={t("task:noAgentFound")}
          dropdownLabel={t("task:agentProfile2")}
          disabled={state.isSubmitting || state.profilesStatus !== "success"}
          touchTarget={isTouchSurface}
          triggerClassName={cn(isTouchSurface ? "h-11 w-full" : "h-7 w-full", "min-w-0 flex-1")}
          testId={`change-workflow-profile-selector-${row.sourceProfileId}`}
          popoverPortal
        />
        <Button
          type="button"
          variant="outline"
          className={cn("shrink-0", isTouchSurface ? "min-h-11" : "h-7")}
          disabled={state.isSubmitting || row.replacementProfileId === row.sourceProfileId}
          onClick={() => state.setOverride(row.sourceProfileId, row.sourceProfileId)}
          data-testid={`change-workflow-profile-reset-${row.sourceProfileId}`}
        >
          {t("task:changeWorkflowResetProfile")}
        </Button>
      </div>
      {invalid && (
        <p role="alert" className="text-xs text-destructive">
          {t("task:changeWorkflowProfileUnavailable")}
        </p>
      )}
    </div>
  );
}

function ConversationSection({ steps }: { steps: readonly WorkflowStepDTO[] }) {
  const { t } = useTranslation();
  const relationships = conversationRelationships(steps);
  if (relationships.length === 0) return null;
  return (
    <section
      className="grid gap-2 rounded-md bg-muted/40 p-3"
      aria-labelledby="change-workflow-conversations-heading"
    >
      <h4 id="change-workflow-conversations-heading" className="text-sm font-medium">
        {t("task:changeWorkflowConversationSection")}
      </h4>
      {relationships.map((relationship) => (
        <p key={relationship.id} className="break-words text-xs text-muted-foreground">
          {relationship.kind === "initial"
            ? t("task:changeWorkflowInitialConversation", { step: relationship.stepName })
            : t("task:changeWorkflowStepConversation", {
                step: relationship.stepName,
                target: relationship.targetName,
              })}
        </p>
      ))}
    </section>
  );
}

export function ChangeWorkflowAgentSection({
  state,
  isTouchSurface,
}: { state: ChangeWorkflowState } & TouchProps) {
  const { t } = useTranslation();
  if (!state.selectedWorkflowId) return null;
  const steps = state.snapshot?.steps ?? [];
  return (
    <section
      className="grid gap-3 border-t border-border/70 pt-3"
      aria-labelledby="change-workflow-agents-heading"
    >
      <div className="grid gap-1">
        <h3 id="change-workflow-agents-heading" className="text-sm font-semibold">
          {t("task:changeWorkflowAgentSection")}
        </h3>
        <p className="text-xs text-muted-foreground">{t("task:changeWorkflowAgentScope")}</p>
      </div>
      {state.profilesStatus === "loading" && (
        <LoadingStatus label={t("task:changeWorkflowLoadingProfiles")} />
      )}
      {state.profilesStatus === "error" && (
        <LoadError
          label={t("task:changeWorkflowLoadProfilesFailed")}
          onRetry={state.retryProfiles}
        />
      )}
      {state.profilesStatus === "success" && state.rows.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("task:changeWorkflowNoFixedProfiles")}</p>
      )}
      {state.profilesStatus === "success" &&
        state.rows.map((row) => (
          <WorkflowAgentRow
            key={row.sourceProfileId}
            state={state}
            row={row}
            isTouchSurface={isTouchSurface}
          />
        ))}
      {state.snapshotStatus === "success" && <ConversationSection steps={steps} />}
      {state.task?.workflow_agent_overrides?.steps.length ? (
        <p className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-xs">
          {t("task:changeWorkflowReplacePreviousOverrides")}
        </p>
      ) : null}
    </section>
  );
}

export function ChangeWorkflowEntryPreview({
  state,
  isTouchSurface,
}: { state: ChangeWorkflowState } & TouchProps) {
  const { t } = useTranslation();
  if (
    state.snapshotStatus !== "success" ||
    !state.selectedStepId ||
    !state.task ||
    !state.workflowChange
  )
    return null;
  return (
    <section
      className="grid gap-2 border-t border-border/70 pt-3"
      aria-labelledby="change-workflow-preview-heading"
    >
      <h3 id="change-workflow-preview-heading" className="text-sm font-semibold">
        {t("task:changeWorkflowEntryPreview")}
      </h3>
      <WorkflowMovePreviewFooter
        target={{
          taskId: state.task.id,
          workflowId: state.selectedWorkflowId,
          workflowStepId: state.selectedStepId,
        }}
        workflowChange={state.workflowChange}
        isTouchSurface={isTouchSurface}
      />
    </section>
  );
}

export function ChangeWorkflowSubmitError({ state }: { state: ChangeWorkflowState }) {
  const { t } = useTranslation();
  const error = state.submitError;
  if (!error) return null;
  let message = t("task:changeWorkflowFailed");
  if (error.code === "workflow_change_conflict") message = t("task:changeWorkflowSourceChanged");
  if (error.code === "workflow_change_uncertain") message = t("task:changeWorkflowUncertain");
  return (
    <div
      role="alert"
      className="grid justify-items-start gap-2 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm"
    >
      <span>{message}</span>
      {error.code === "workflow_change_uncertain" && (
        <RetryButton
          onClick={state.retryTask}
          label={t("task:changeWorkflowRefreshTask")}
          testId="change-workflow-refresh-task"
        />
      )}
    </div>
  );
}

export function ChangeWorkflowActionFooter({
  state,
  isTouchSurface,
  onOpenChange,
}: TouchProps & { state: ChangeWorkflowState; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-end gap-2 border-t border-border/70 px-5 py-3",
        isTouchSurface &&
          "justify-stretch px-4 pb-[max(12px,env(safe-area-inset-bottom,0px))] pt-3",
      )}
    >
      <Button
        type="button"
        variant="outline"
        className={cn(isTouchSurface && "min-h-12 flex-1")}
        disabled={state.isSubmitting}
        onClick={() => onOpenChange(false)}
        data-testid="change-workflow-cancel"
      >
        {t("task:changeWorkflowCancel")}
      </Button>
      <Button
        type="submit"
        className={cn(isTouchSurface && "min-h-12 flex-1")}
        disabled={!state.canSubmit}
        data-testid="change-workflow-submit"
      >
        {state.isSubmitting && <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />}
        {t("task:changeWorkflowSubmit")}
      </Button>
    </div>
  );
}
