"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconArrowLeft, IconArrowRight } from "@tabler/icons-react";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { coerceCatchUpMax } from "../lib/catch-up-max";
import { useTranslation } from "react-i18next";

type CreateRoutineDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agents: AgentProfile[];
  onSubmit: (data: {
    name: string;
    description: string;
    taskTitle: string;
    taskDescription: string;
    assigneeAgentProfileId: string;
    concurrencyPolicy: string;
    catchUpPolicy: string;
    catchUpMax: number;
    triggerKind: string;
    cronExpression: string;
    timezone: string;
  }) => Promise<boolean>;
};

type RoutineFormState = {
  name: string;
  description: string;
  taskTitle: string;
  taskDesc: string;
  assignee: string;
  concurrency: string;
  catchUpPolicy: string;
  catchUpMax: string;
  triggerKind: string;
  cronExpr: string;
  timezone: string;
};

const INITIAL_ROUTINE_STATE: RoutineFormState = {
  name: "",
  description: "",
  taskTitle: "",
  taskDesc: "",
  assignee: "",
  concurrency: "coalesce_if_active",
  catchUpPolicy: "summarize_missed",
  catchUpMax: "25",
  triggerKind: "cron",
  cronExpr: "",
  timezone: "UTC",
};

const STEP_COUNT = 3;
// Catalog keys, not titles — module scope freezes a `t()` at the boot locale.
const STEP_TITLE_KEYS = ["office:stepDetails", "office:stepTaskTemplate", "office:stepSchedule"];

function dotColor(index: number, current: number): string {
  if (index === current) return "bg-primary";
  if (index < current) return "bg-primary/50";
  return "bg-muted";
}

function StepIndicator({ current }: { current: number }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-center gap-2 pt-1">
      {STEP_TITLE_KEYS.map((titleKey, i) => (
        <div
          key={titleKey}
          className={`h-2 w-2 rounded-full transition-colors ${dotColor(i, current)}`}
          aria-label={t("office:stepNumberTitle", { number: i + 1, title: t(titleKey) })}
        />
      ))}
    </div>
  );
}

function StepDetails({
  state,
  agents,
  onUpdate,
}: {
  state: RoutineFormState;
  agents: AgentProfile[];
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="routine-name">{t("office:name")}</Label>
        <Input
          id="routine-name"
          value={state.name}
          onChange={(e) => onUpdate({ name: e.target.value })}
          placeholder={t("office:dailyDepUpdate")}
          className="mt-1.5"
          autoFocus
        />
      </div>
      <div>
        <Label htmlFor="routine-description">{t("office:description")}</Label>
        <Textarea
          id="routine-description"
          value={state.description}
          onChange={(e) => onUpdate({ description: e.target.value })}
          rows={2}
          className="mt-1.5"
        />
      </div>
      <div>
        <Label>{t("office:assignee")}</Label>
        <Select value={state.assignee} onValueChange={(v) => onUpdate({ assignee: v })}>
          <SelectTrigger className="cursor-pointer mt-1.5">
            <SelectValue placeholder={t("office:selectAgent")} />
          </SelectTrigger>
          <SelectContent>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1.5">
          {t("office:agentThatPicksUpRunsTriggered")}
        </p>
      </div>
    </div>
  );
}

function StepTaskTemplate({
  state,
  onUpdate,
}: {
  state: RoutineFormState;
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="routine-task-title">{t("office:taskTitleTemplate")}</Label>
        <Input
          id="routine-task-title"
          value={state.taskTitle}
          onChange={(e) => onUpdate({ taskTitle: e.target.value })}
          // Literal, not a key: this placeholder IS the template syntax the field
          // accepts. Routed through `t()` it becomes an i18next interpolation and
          // both tokens resolve to nothing.
          placeholder="{{name}} - {{date}}"
          className="mt-1.5"
        />
        <p className="text-xs text-muted-foreground mt-1.5">
          {t("office:titleForAutoCreatedTasksUse")}
        </p>
      </div>
      <div>
        <Label htmlFor="routine-task-desc">{t("office:taskDescriptionTemplate")}</Label>
        <Textarea
          id="routine-task-desc"
          value={state.taskDesc}
          onChange={(e) => onUpdate({ taskDesc: e.target.value })}
          rows={4}
          className="mt-1.5"
        />
        <p className="text-xs text-muted-foreground mt-1.5">
          {t("office:instructionsTheAgentReceivesWhenThis")}
        </p>
      </div>
    </div>
  );
}

function TriggerFields({
  state,
  onUpdate,
}: {
  state: RoutineFormState;
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <Label>{t("office:triggerType")}</Label>
          <Select value={state.triggerKind} onValueChange={(v) => onUpdate({ triggerKind: v })}>
            <SelectTrigger className="cursor-pointer mt-1.5">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="cron" className="cursor-pointer">
                {t("office:cron")}
              </SelectItem>
              <SelectItem value="webhook" className="cursor-pointer">
                {t("office:webhook")}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {state.triggerKind === "cron" && (
          <div>
            <Label htmlFor="routine-cron">{t("office:cronExpressionTitleCase")}</Label>
            <Input
              id="routine-cron"
              value={state.cronExpr}
              onChange={(e) => onUpdate({ cronExpr: e.target.value })}
              placeholder="0 9 * * *"
              className="mt-1.5"
            />
          </div>
        )}
      </div>
      {state.triggerKind === "cron" && (
        <>
          <p className="text-xs text-muted-foreground -mt-2">
            {/*
              The cron expression is SYNTAX, not copy: it travels as a value so a
              translator cannot reword it into something no parser accepts.
              Guarded by app/office/office-cron-i18n.test.ts.
            */}
            {t("office:standardCronExpressionExample", { cron: "0 9 * * MON" })}
          </p>
          <div>
            <Label htmlFor="routine-timezone">{t("office:timezone")}</Label>
            <Input
              id="routine-timezone"
              value={state.timezone}
              onChange={(e) => onUpdate({ timezone: e.target.value })}
              placeholder="UTC"
              className="mt-1.5"
            />
          </div>
        </>
      )}
    </div>
  );
}

function PolicyFields({
  state,
  onUpdate,
}: {
  state: RoutineFormState;
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-4">
      <div>
        <Label>{t("office:concurrency")}</Label>
        <Select value={state.concurrency} onValueChange={(v) => onUpdate({ concurrency: v })}>
          <SelectTrigger className="cursor-pointer mt-1.5">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="skip_if_active" className="cursor-pointer">
              {t("office:skipIfActive")}
            </SelectItem>
            <SelectItem value="coalesce_if_active" className="cursor-pointer">
              {t("office:coalesce")}
            </SelectItem>
            <SelectItem value="always_create" className="cursor-pointer">
              {t("office:alwaysCreate")}
            </SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1.5">
          {t("office:whatHappensIfThePreviousRun")}
        </p>
      </div>
      <div>
        <Label>{t("office:catchUpPolicy")}</Label>
        <Select value={state.catchUpPolicy} onValueChange={(v) => onUpdate({ catchUpPolicy: v })}>
          <SelectTrigger className="cursor-pointer mt-1.5">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="summarize_missed" className="cursor-pointer">
              {t("office:summarizeMissed")}
            </SelectItem>
            <SelectItem value="skip_missed" className="cursor-pointer">
              {t("office:skipMissed")}
            </SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1.5">
          {t("office:whatHappensToTicksMissedWhile")}
        </p>
      </div>
      {state.catchUpPolicy === "summarize_missed" && (
        <div className="col-span-2">
          <Label htmlFor="routine-catchup-max">{t("office:catchUpMax")}</Label>
          <Input
            id="routine-catchup-max"
            type="number"
            min={1}
            value={state.catchUpMax}
            onChange={(e) => onUpdate({ catchUpMax: e.target.value })}
            onBlur={(e) => onUpdate({ catchUpMax: String(coerceCatchUpMax(e.target.value)) })}
            className="mt-1.5"
          />
          <p className="text-xs text-muted-foreground mt-1.5">
            {t("office:beyondThisCountMissedTicksAreNotCounted")}
          </p>
        </div>
      )}
    </div>
  );
}

function StepSchedule({
  state,
  onUpdate,
}: {
  state: RoutineFormState;
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  return (
    <div className="space-y-6">
      <TriggerFields state={state} onUpdate={onUpdate} />
      <PolicyFields state={state} onUpdate={onUpdate} />
    </div>
  );
}

function canAdvance(step: number, state: RoutineFormState): boolean {
  if (step === 0) return state.name.trim() !== "" && state.assignee !== "";
  if (step === 2 && state.triggerKind === "cron") return state.cronExpr.trim() !== "";
  return true;
}

function buildSubmitPayload(state: RoutineFormState) {
  return {
    name: state.name,
    description: state.description,
    taskTitle: state.taskTitle,
    taskDescription: state.taskDesc,
    assigneeAgentProfileId: state.assignee,
    concurrencyPolicy: state.concurrency,
    catchUpPolicy: state.catchUpPolicy,
    catchUpMax: coerceCatchUpMax(state.catchUpMax),
    triggerKind: state.triggerKind,
    cronExpression: state.cronExpr,
    timezone: state.timezone,
  };
}

// Owns the dialog's step/form state plus submission, keeping
// CreateRoutineDialog itself under the per-function line ceiling.
function useCreateRoutineDialogState(
  onOpenChange: (open: boolean) => void,
  onSubmit: CreateRoutineDialogProps["onSubmit"],
) {
  const [step, setStep] = useState(0);
  const [state, setState] = useState<RoutineFormState>(INITIAL_ROUTINE_STATE);
  // Guards against a second `handleSubmit` firing (double-click, or a
  // repeated Enter activation per the dialog's Enter-to-confirm behavior)
  // while the first `onSubmit` call is still in flight — the backend has no
  // create idempotency guard, so two concurrent submits persist two routines.
  const [submitting, setSubmitting] = useState(false);
  const update = (patch: Partial<RoutineFormState>) => setState((prev) => ({ ...prev, ...patch }));

  function handleOpenChange(next: boolean) {
    if (!next) {
      setState(INITIAL_ROUTINE_STATE);
      setStep(0);
    }
    onOpenChange(next);
  }

  async function handleSubmit() {
    if (submitting) return;
    setSubmitting(true);
    try {
      const succeeded = await onSubmit(buildSubmitPayload(state));
      // A rejected create leaves the dialog open (per onSubmit's contract) for
      // the user to correct and retry; resetting the form on that path would
      // silently discard what they just typed.
      if (succeeded) handleOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  }

  return { step, setStep, state, update, submitting, handleOpenChange, handleSubmit };
}

function StepContent({
  step,
  state,
  agents,
  onUpdate,
}: {
  step: number;
  state: RoutineFormState;
  agents: AgentProfile[];
  onUpdate: (patch: Partial<RoutineFormState>) => void;
}) {
  if (step === 0) return <StepDetails state={state} agents={agents} onUpdate={onUpdate} />;
  if (step === 1) return <StepTaskTemplate state={state} onUpdate={onUpdate} />;
  return <StepSchedule state={state} onUpdate={onUpdate} />;
}

export function CreateRoutineDialog({
  open,
  onOpenChange,
  agents,
  onSubmit,
}: CreateRoutineDialogProps) {
  const { t } = useTranslation();
  const { step, setStep, state, update, submitting, handleOpenChange, handleSubmit } =
    useCreateRoutineDialogState(onOpenChange, onSubmit);

  const isLast = step === STEP_COUNT - 1;
  const advanceEnabled = canAdvance(step, state);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office:createRoutine")}</DialogTitle>
          <p className="text-sm text-muted-foreground">
            {t("office:stepXOfYTitle", {
              number: step + 1,
              total: STEP_COUNT,
              title: t(STEP_TITLE_KEYS[step]),
            })}
          </p>
          <StepIndicator current={step} />
        </DialogHeader>
        <div className="pt-2">
          <StepContent step={step} state={state} agents={agents} onUpdate={update} />
        </div>
        <DialogFooter className="sm:justify-between">
          <div>
            {step > 0 && (
              <Button
                variant="ghost"
                onClick={() => setStep((s) => s - 1)}
                className="cursor-pointer"
              >
                <IconArrowLeft className="h-4 w-4 mr-1" />
                {t("common:back")}
              </Button>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              onClick={() => handleOpenChange(false)}
              className="cursor-pointer"
            >
              {t("common:cancel")}
            </Button>
            {isLast ? (
              <Button
                onClick={handleSubmit}
                disabled={!advanceEnabled || submitting}
                className="cursor-pointer"
              >
                {t("office:create")}
              </Button>
            ) : (
              <Button
                onClick={() => setStep((s) => s + 1)}
                disabled={!advanceEnabled}
                className="cursor-pointer"
              >
                {t("common:next")}
                <IconArrowRight className="h-4 w-4 ml-1" />
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
