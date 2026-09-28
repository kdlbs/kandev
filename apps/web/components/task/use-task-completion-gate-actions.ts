"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  setTaskCompletionCriteria,
  verifyTaskCompletionCriterion,
  type TaskCompletionEvidenceSubject,
  type TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";
import {
  completionDraftWeakensUnmetCriteria,
  toCompletionDraft,
  type CompletionDraftCriterion,
} from "./task-completion-gate-criteria";

type CriteriaActionsInput = {
  taskId: string;
  snapshot: TaskCompletionGateSnapshot | null;
  onRefresh: () => Promise<void>;
  onError: (message: string) => void;
};

export function useCompletionCriteriaActions(input: CriteriaActionsInput) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<CompletionDraftCriterion[]>([]);
  const [saving, setSaving] = useState(false);
  const [confirmationReason, setConfirmationReason] = useState("");
  const [confirmWeakening, setConfirmWeakening] = useState(false);

  useEffect(() => {
    setDraft(toCompletionDraft(input.snapshot));
    setConfirmationReason("");
    setConfirmWeakening(false);
  }, [input.snapshot]);

  const weakening = useMemo(
    () => completionDraftWeakensUnmetCriteria(input.snapshot, draft),
    [draft, input.snapshot],
  );
  const changed = useMemo(
    () => JSON.stringify(toCompletionDraft(input.snapshot)) !== JSON.stringify(draft),
    [draft, input.snapshot],
  );

  const save = useCallback(async () => {
    const snapshot = input.snapshot;
    if (!snapshot || saving || (weakening && (!confirmWeakening || !confirmationReason.trim()))) {
      return;
    }
    setSaving(true);
    try {
      await setTaskCompletionCriteria(input.taskId, {
        expected_revision: snapshot.revision,
        criteria: draft,
        ...(weakening
          ? {
              human_confirmation: {
                expected_revision: snapshot.revision,
                reason: confirmationReason.trim(),
              },
            }
          : {}),
      });
      await input.onRefresh();
      setConfirmationReason("");
      setConfirmWeakening(false);
    } catch {
      input.onError(t("task:completionGateSaveError"));
    } finally {
      setSaving(false);
    }
  }, [confirmWeakening, confirmationReason, draft, input, saving, t, weakening]);

  return {
    changed,
    confirmWeakening,
    confirmationReason,
    draft,
    onDraftChange: setDraft,
    onConfirmWeakeningChange: setConfirmWeakening,
    onConfirmationReasonChange: setConfirmationReason,
    onSave: save,
    saving,
    weakening,
  };
}

type EvidenceActionsInput = {
  taskId: string;
  snapshot: TaskCompletionGateSnapshot | null;
  onRefresh: () => Promise<void>;
  onError: (message: string) => void;
};

export function useCompletionEvidenceActions(input: EvidenceActionsInput) {
  const { t } = useTranslation();
  const [savingCriterionId, setSavingCriterionId] = useState<string | null>(null);
  const save = useCallback(
    async (
      criterionId: string,
      subject: TaskCompletionEvidenceSubject,
      revision: string,
      summary: string,
      reference: string,
    ) => {
      const snapshot = input.snapshot;
      if (!snapshot || !revision.trim() || savingCriterionId !== null) return;
      setSavingCriterionId(criterionId);
      try {
        await verifyTaskCompletionCriterion(input.taskId, criterionId, {
          expected_revision: snapshot.revision,
          evidence: {
            subject: { ...subject, revision: revision.trim() },
            summary: summary.trim(),
            reference: reference.trim(),
          },
        });
        await input.onRefresh();
      } catch {
        input.onError(t("task:completionGateEvidenceError"));
      } finally {
        setSavingCriterionId(null);
      }
    },
    [input, savingCriterionId, t],
  );
  return { onSave: save, savingCriterionId };
}

type OverrideActionsInput = {
  snapshot: TaskCompletionGateSnapshot | null;
  onOverride: (stepId: string, reason: string, revision: number) => Promise<boolean>;
};

export function useCompletionOverrideActions(input: OverrideActionsInput) {
  const [selectedStep, setSelectedStep] = useState("");
  const [reason, setReason] = useState("");
  const [saving, setSaving] = useState(false);
  const submit = useCallback(async () => {
    const snapshot = input.snapshot;
    if (!snapshot || !selectedStep || !reason.trim() || saving) return;
    setSaving(true);
    try {
      if (await input.onOverride(selectedStep, reason.trim(), snapshot.revision)) setReason("");
    } finally {
      setSaving(false);
    }
  }, [input, reason, saving, selectedStep]);
  return {
    onReasonChange: setReason,
    onStepChange: setSelectedStep,
    reason,
    saving,
    selectedStep,
    submit,
  };
}
