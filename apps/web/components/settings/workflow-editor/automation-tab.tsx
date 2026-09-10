"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { WorkflowStep } from "@/lib/types/http";
import { Button } from "@kandev/ui/button";
import {
  createWorkflowAction,
  type WorkflowActionRecord,
  type WorkflowLifecycleTrigger,
} from "@/lib/workflows/workflow-action-catalog";
import {
  addWorkflowAction,
  moveWorkflowAction,
  removeWorkflowAction,
  repairWorkflowActionSelection,
  updateWorkflowAction,
} from "@/components/settings/workflow-step-mutations";
import { WorkflowActionList } from "./action-list";
import { WorkflowActionEditor } from "./action-editor";

const AUTOMATION_TRIGGERS: readonly WorkflowLifecycleTrigger[] = [
  "on_enter",
  "on_turn_start",
  "on_turn_complete",
  "on_exit",
  "on_children_completed",
];

export type WorkflowActionSelection = {
  trigger: WorkflowLifecycleTrigger;
  index: number;
};

export type AutomationTabProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  readOnly: boolean;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  focusedAction?: WorkflowActionSelection | null;
  mobile?: boolean;
  onFocusAction?: (selection: WorkflowActionSelection | null, mode?: "push" | "replace") => void;
};

export function AutomationTab({
  step,
  steps,
  readOnly,
  onUpdate,
  focusedAction,
  mobile = false,
  onFocusAction,
}: AutomationTabProps) {
  const [selection, setSelection] = useState<WorkflowActionSelection | null>(null);
  const selectedActions = selection ? actionsFor(step, selection.trigger) : [];

  useEffect(() => {
    setSelection(focusedAction ?? null);
  }, [focusedAction]);

  useEffect(() => {
    if (!selection) return;
    const actions = actionsFor(step, selection.trigger);
    if (selection.index >= actions.length) {
      const repaired = repairWorkflowActionSelection(selection.index, actions.length);
      const nextSelection = repaired === null ? null : { ...selection, index: repaired };
      setSelection(nextSelection);
      onFocusAction?.(nextSelection);
    }
  }, [onFocusAction, selection, step]);

  const updateTrigger = (_trigger: WorkflowLifecycleTrigger, next: WorkflowStep) => {
    onUpdate({ events: next.events });
  };

  const handleAdd = (trigger: WorkflowLifecycleTrigger, type: string) => {
    const next = addWorkflowAction(step, trigger, createWorkflowAction(trigger, type));
    updateTrigger(trigger, next);
    selectAction({ trigger, index: actionsFor(next, trigger).length - 1 });
  };

  const handleChange = (
    trigger: WorkflowLifecycleTrigger,
    index: number,
    updates: Partial<WorkflowActionRecord>,
  ) => {
    updateTrigger(trigger, updateWorkflowAction(step, trigger, index, updates));
  };

  const handleMove = (trigger: WorkflowLifecycleTrigger, index: number, direction: -1 | 1) => {
    const nextIndex = index + direction;
    updateTrigger(trigger, moveWorkflowAction(step, trigger, index, nextIndex));
    selectAction({ trigger, index: nextIndex }, "replace");
  };

  const handleRemove = (trigger: WorkflowLifecycleTrigger, index: number) => {
    const next = removeWorkflowAction(step, trigger, index);
    updateTrigger(trigger, next);
    const repaired = repairWorkflowActionSelection(index, actionsFor(next, trigger).length);
    selectAction(repaired === null ? null : { trigger, index: repaired }, "replace");
  };

  const selectAction = (
    nextSelection: WorkflowActionSelection | null,
    mode: "push" | "replace" = "push",
  ) => {
    setSelection(nextSelection);
    onFocusAction?.(nextSelection, mode);
  };

  const clearSelection = () => selectAction(null, "replace");

  if (selection && selectedActions[selection.index]) {
    const action = selectedActions[selection.index];
    return (
      <FocusedActionEditor
        action={action}
        actionIndex={selection.index}
        actionCount={selectedActions.length}
        trigger={selection.trigger}
        steps={steps}
        readOnly={readOnly}
        onBack={clearSelection}
        onChange={(updates) => handleChange(selection.trigger, selection.index, updates)}
        onMove={(direction) => handleMove(selection.trigger, selection.index, direction)}
        onRemove={() => handleRemove(selection.trigger, selection.index)}
      />
    );
  }

  return (
    <div className="space-y-4" data-testid="workflow-automation-tab">
      {AUTOMATION_TRIGGERS.map((trigger) => {
        const actions = actionsFor(step, trigger);
        return (
          <WorkflowActionList
            key={trigger}
            trigger={trigger}
            actions={actions}
            readOnly={readOnly}
            onSelect={(index) => selectAction({ trigger, index })}
            onAdd={(type) => handleAdd(trigger, type)}
            mobile={mobile}
          />
        );
      })}
      {selection && selectedActions[selection.index] && (
        <span className="sr-only" data-testid="workflow-selected-action">
          {selection.trigger}:{selection.index}
        </span>
      )}
    </div>
  );
}

function actionsFor(step: WorkflowStep, trigger: WorkflowLifecycleTrigger): WorkflowActionRecord[] {
  return (step.events?.[trigger] ?? []) as unknown as WorkflowActionRecord[];
}

function FocusedActionEditor({
  action,
  actionIndex,
  actionCount,
  trigger,
  steps,
  readOnly,
  onBack,
  onChange,
  onMove,
  onRemove,
}: {
  action: WorkflowActionRecord;
  actionIndex: number;
  actionCount: number;
  trigger: WorkflowLifecycleTrigger;
  steps: WorkflowStep[];
  readOnly: boolean;
  onBack: () => void;
  onChange: (updates: Partial<WorkflowActionRecord>) => void;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4" data-testid="workflow-focused-action-editor">
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer px-2"
        onClick={onBack}
      >
        {t("workflows:backToAutomation")}
      </Button>
      <WorkflowActionEditor
        action={action}
        actionIndex={actionIndex}
        actionCount={actionCount}
        trigger={trigger}
        steps={steps}
        readOnly={readOnly}
        onChange={onChange}
        onMove={onMove}
        onRemove={onRemove}
      />
    </div>
  );
}
