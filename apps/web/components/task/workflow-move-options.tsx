"use client";

import { useCallback, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

export type WorkflowMoveOptionsDraft = {
  resetContext: boolean;
  instructions: string;
  skipStepPrompt: boolean;
};

const EMPTY_DRAFT: WorkflowMoveOptionsDraft = {
  resetContext: false,
  instructions: "",
  skipStepPrompt: false,
};

export function workflowMoveOptionsPayload(
  draft: WorkflowMoveOptionsDraft,
): WorkflowMoveEntryOptions | undefined {
  const payload: WorkflowMoveEntryOptions = {};
  if (draft.resetContext) payload.reset_context = true;
  if (draft.skipStepPrompt) payload.skip_step_prompt = true;
  if (draft.instructions.trim()) payload.instructions = draft.instructions.trim();
  return Object.keys(payload).length > 0 ? payload : undefined;
}

/**
 * Draft state shared by every move-options surface (stepper disclosure,
 * proceed popover, dialog, drawer).
 */
export function useWorkflowMoveOptionsForm() {
  const [draft, setDraft] = useState<WorkflowMoveOptionsDraft>(EMPTY_DRAFT);

  const patchDraft = useCallback((patch: Partial<WorkflowMoveOptionsDraft>) => {
    setDraft((current) => ({ ...current, ...patch }));
  }, []);
  const resetDraft = useCallback(() => setDraft(EMPTY_DRAFT), []);

  return {
    draft,
    patchDraft,
    resetDraft,
  };
}

type WorkflowMoveOptionsFieldsProps = {
  draft: WorkflowMoveOptionsDraft;
  onDraftChange: (patch: Partial<WorkflowMoveOptionsDraft>) => void;
  isTouchSurface: boolean;
  instructionsRows?: number;
};

/**
 * Reset-context, skip-step-prompt, and one-time instructions fields. Font size
 * is inherited from the hosting surface (Dialog, Drawer, and the stepper hover
 * card all share the `text-xs/relaxed` popover base) so the fields never stand
 * out larger than the surrounding chrome. Checkboxes align to the first text
 * line so a wrapping label reads cleanly. The skip-step-prompt consequence is
 * disclosed through an info-icon tooltip (hover on desktop, focus/tap on touch)
 * rather than an always-visible paragraph.
 */
export function WorkflowMoveOptionsFields({
  draft,
  onDraftChange,
  isTouchSurface,
  instructionsRows = 4,
}: WorkflowMoveOptionsFieldsProps) {
  const { t } = useTranslation();
  const rowClass = isTouchSurface ? "flex items-center gap-3 min-h-11" : "flex items-start gap-2.5";
  const checkboxClass = cn("shrink-0", !isTouchSurface && "mt-0.5");
  const hintButtonClass = cn(
    "inline-flex shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
    isTouchSurface ? "size-11" : "mt-0.5 size-4",
  );
  return (
    <div className="grid gap-3.5">
      <label className={rowClass}>
        <Checkbox
          className={checkboxClass}
          checked={draft.resetContext}
          onCheckedChange={(checked) => onDraftChange({ resetContext: checked === true })}
          data-testid="workflow-move-reset-context"
        />
        <span className="leading-snug">{t("task:workflowMoveResetContext")}</span>
      </label>
      <div className={rowClass}>
        <label className="flex min-w-0 flex-1 items-start gap-2.5">
          <Checkbox
            className={checkboxClass}
            checked={draft.skipStepPrompt}
            onCheckedChange={(checked) => onDraftChange({ skipStepPrompt: checked === true })}
            data-testid="workflow-move-skip-step-prompt"
          />
          <span className="leading-snug">{t("task:workflowMoveSkipStepPrompt")}</span>
        </label>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className={hintButtonClass}
              aria-label={t("workflows:moreInformation")}
              data-testid="workflow-move-skip-step-prompt-hint"
            >
              <IconInfoCircle className="size-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-xs leading-relaxed">
            {t("task:workflowMoveSkipStepPromptHint")}
          </TooltipContent>
        </Tooltip>
      </div>
      <label className="grid gap-1.5">
        <span>{t("task:workflowMoveInstructions")}</span>
        <Textarea
          value={draft.instructions}
          onChange={(event) => onDraftChange({ instructions: event.target.value })}
          placeholder={t("task:workflowMoveInstructionsPlaceholder")}
          rows={instructionsRows}
          data-testid="workflow-move-instructions"
        />
      </label>
    </div>
  );
}

/** Result contract for move-options submits: `false` keeps the form open. */
export type WorkflowMoveOptionsSubmit = (
  options: WorkflowMoveEntryOptions | undefined,
) => boolean | void | Promise<boolean | void>;

type WorkflowMoveOptionsFormProps = {
  isMoving: boolean;
  isTouchSurface: boolean;
  instructionsRows?: number;
  onSubmit: WorkflowMoveOptionsSubmit;
  onCancel?: () => void;
};

type WorkflowMoveOptionsStateProps = {
  onSubmit: WorkflowMoveOptionsSubmit;
  isMoving: boolean;
};

function useWorkflowMoveOptionsFormState({ onSubmit, isMoving }: WorkflowMoveOptionsStateProps) {
  const { draft, patchDraft, resetDraft } = useWorkflowMoveOptionsForm();
  const [submitting, setSubmitting] = useState(false);
  const busy = isMoving || submitting;

  const submit = async () => {
    if (busy) return;
    setSubmitting(true);
    try {
      const result = await onSubmit(workflowMoveOptionsPayload(draft));
      if (result !== false) resetDraft();
    } finally {
      setSubmitting(false);
    }
  };

  return {
    draft,
    patchDraft,
    busy,
    submit,
  };
}

function WorkflowMoveOptionsActions({
  busy,
  isTouchSurface,
  onCancel,
  onSubmit,
}: {
  busy: boolean;
  isTouchSurface: boolean;
  onCancel?: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  const touchTargetClass = isTouchSurface ? "min-h-11" : "h-7";
  return (
    <div className="flex justify-end gap-2">
      {onCancel && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={touchTargetClass}
          onClick={onCancel}
        >
          {t("common:cancel")}
        </Button>
      )}
      <Button
        type="button"
        size="sm"
        className={touchTargetClass}
        disabled={busy}
        onClick={onSubmit}
        data-testid="workflow-move-submit"
      >
        {busy ? t("task:moving") : t("task:workflowMoveApply")}
      </Button>
    </div>
  );
}

/**
 * Complete self-contained options form: fields plus the Move submit action.
 * A submit that resolves to `false` (failed move) keeps the draft and the
 * form open so nothing the user typed is lost.
 */
export function WorkflowMoveOptionsForm({
  isMoving,
  isTouchSurface,
  instructionsRows,
  onSubmit,
  onCancel,
}: WorkflowMoveOptionsFormProps) {
  const { draft, patchDraft, busy, submit } = useWorkflowMoveOptionsFormState({
    onSubmit,
    isMoving,
  });
  return (
    <div className="grid gap-4">
      <WorkflowMoveOptionsFields
        draft={draft}
        onDraftChange={patchDraft}
        isTouchSurface={isTouchSurface}
        instructionsRows={instructionsRows}
      />
      <WorkflowMoveOptionsActions
        busy={busy}
        isTouchSurface={isTouchSurface}
        onCancel={onCancel}
        onSubmit={() => void submit()}
      />
    </div>
  );
}

type WorkflowMoveOptionsProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetStepName: string;
  isMoving?: boolean;
  onSubmit: WorkflowMoveOptionsSubmit;
};

/** Fine-pointer Dialog wrapper around the shared form. */
export function WorkflowMoveDialog({
  open,
  onOpenChange,
  targetStepName,
  isMoving = false,
  onSubmit,
}: WorkflowMoveOptionsProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" data-testid="workflow-move-options">
        <DialogHeader>
          <DialogTitle>{t("task:workflowMoveOptionsTitle", { step: targetStepName })}</DialogTitle>
          <DialogDescription>{t("task:workflowMoveOptionsDescription")}</DialogDescription>
        </DialogHeader>
        <WorkflowMoveOptionsForm
          isMoving={isMoving}
          isTouchSurface={false}
          onSubmit={onSubmit}
          onCancel={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

/** Touch Drawer wrapper around the shared form. */
export function WorkflowMoveOptions({
  open,
  onOpenChange,
  targetStepName,
  isMoving = false,
  onSubmit,
}: WorkflowMoveOptionsProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  if (!usesTouchDrawer) return null;
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent className="!top-0 !bottom-auto !mt-0 !h-dvh !max-h-dvh min-h-0 overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
        <DrawerHeader className="shrink-0 text-left">
          <DrawerTitle>{t("task:workflowMoveOptionsTitle", { step: targetStepName })}</DrawerTitle>
          <DrawerDescription>{t("task:workflowMoveOptionsDescription")}</DrawerDescription>
        </DrawerHeader>
        <div
          className="min-h-0 flex-1 overflow-y-auto px-4"
          data-vaul-no-drag
          data-testid="workflow-move-options"
        >
          <WorkflowMoveOptionsForm
            isMoving={isMoving}
            isTouchSurface
            onSubmit={onSubmit}
            onCancel={() => onOpenChange(false)}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}
