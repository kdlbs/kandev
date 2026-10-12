"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useGoal } from "@/hooks/domains/coordinator/use-goal";
import { GoalFormFields } from "./goal-form-fields";
import { GoalMeasuresList } from "./goal-measures";
import { useGoalEditor } from "./use-goal-editor";

type GoalSectionProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
};

type MarkMetDialogProps = {
  open: boolean;
  name: string;
  dirty: boolean;
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

function MarkMetDialog({ open, name, dirty, busy, onOpenChange, onConfirm }: MarkMetDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid="goal-mark-met-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("coordinator:goalMarkMetTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("coordinator:goalMarkMetDescription", { name })}
            {dirty && ` ${t("coordinator:goalMarkMetDiscard")}`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy} className="cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <Button
            type="button"
            disabled={busy}
            data-testid="goal-mark-met-confirm"
            className="cursor-pointer"
            onClick={onConfirm}
          >
            {t("coordinator:goalMarkMetConfirm")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function GoalSection({ workspaceId, coordinatorId, canManage }: GoalSectionProps) {
  const { t } = useTranslation();
  const { data, status, reload, retry } = useGoal(workspaceId, coordinatorId);
  const editor = useGoalEditor({ workspaceId, coordinatorId, canManage, data, reload });
  const [confirmOpen, setConfirmOpen] = useState(false);
  const { active, form, busy } = editor;

  if (status === "loading") {
    return <div className="h-32 animate-pulse rounded-md bg-muted" data-testid="goal-loading" />;
  }
  if (status === "error" || !data) {
    return (
      <div className="space-y-3 text-sm text-muted-foreground" data-testid="goal-error">
        <p>{t("coordinator:loadError")}</p>
        <Button type="button" variant="outline" className="cursor-pointer" onClick={retry}>
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (!active && !canManage) {
    return <p className="text-sm text-muted-foreground">{t("coordinator:goalNone")}</p>;
  }

  return (
    <div className="space-y-4" data-testid="goal-section">
      {editor.notice && (
        <p role="alert" className="text-sm text-destructive" data-testid="goal-notice">
          {editor.notice}
        </p>
      )}
      <GoalFormFields
        form={form}
        disabled={!canManage || busy}
        fieldError={editor.fieldError}
        onChange={editor.setForm}
        onToggle={editor.toggleCriterion}
      />
      {active && data.measures && (
        <GoalMeasuresList measures={data.measures} setAt={active.set_at} />
      )}
      {canManage && !active && (
        <Button
          type="button"
          data-testid="goal-set"
          disabled={busy || form.name.trim() === ""}
          className="min-h-12 cursor-pointer sm:min-h-9"
          onClick={() => void editor.saveGoal().catch(() => undefined)}
        >
          {t("coordinator:goalSetGoal")}
        </Button>
      )}
      {canManage && active && (
        <>
          <Button
            type="button"
            variant="outline"
            data-testid="goal-mark-met"
            disabled={busy}
            className="min-h-12 cursor-pointer sm:min-h-9"
            onClick={() => setConfirmOpen(true)}
          >
            {t("coordinator:goalMarkMet")}
          </Button>
          <MarkMetDialog
            open={confirmOpen}
            name={active.name}
            dirty={editor.dirty}
            busy={busy}
            onOpenChange={setConfirmOpen}
            onConfirm={() => void editor.markMet().then(() => setConfirmOpen(false))}
          />
        </>
      )}
    </div>
  );
}
