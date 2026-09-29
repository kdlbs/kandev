"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import { markGoalMet, putGoal, setGoalCriterionDone } from "@/lib/api/domains/coordinator-api";
import type { Goal, GoalResponse } from "@/lib/api/domains/coordinator-api";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { toast } from "@/lib/toast/sonner";
import {
  buildPutGoalRequest,
  EMPTY_GOAL_FORM,
  goalFormFromGoal,
  isGoalFormDirty,
  withCriterionDoneState,
  withServerDoneStates,
  type GoalFormState,
} from "@/lib/coordinators/goal-form";

export type GoalFieldError = { field: string | null; message: string };

function fieldErrorOf(error: unknown): GoalFieldError | null {
  if (!(error instanceof ApiError) || error.status !== 400) return null;
  const body = error.body as { error?: unknown; field?: unknown } | null;
  const message = typeof body?.error === "string" ? body.error : error.message;
  return { field: typeof body?.field === "string" ? body.field : null, message };
}

function isStaleCriterionId(error: GoalFieldError): boolean {
  return error.field !== null && /^criteria\[\d+\]\.id$/.test(error.field);
}

// Mirrors the stored goal into the form: a different goal (or a forced
// reload) replaces it, a clean form follows the server, and a dirty form only
// takes the server's done states.
function useAdoptedGoalForm(
  data: GoalResponse | null,
  inFlight: React.MutableRefObject<Set<string>>,
) {
  const active = data?.active ?? null;
  const [form, setForm] = useState<GoalFormState>(EMPTY_GOAL_FORM);
  const [saved, setSaved] = useState<GoalFormState>(EMPTY_GOAL_FORM);
  const adoptedRef = useRef<string | null | undefined>(undefined);
  const forceAdoptRef = useRef(false);
  const formRef = useRef(form);
  formRef.current = form;
  const savedRef = useRef(saved);
  savedRef.current = saved;

  useEffect(() => {
    if (!data) return;
    const id = active?.id ?? null;
    const changedGoal = adoptedRef.current !== id;
    const clean = !isGoalFormDirty(formRef.current, savedRef.current);
    if (changedGoal || forceAdoptRef.current || (clean && active)) {
      const next = active ? goalFormFromGoal(active) : EMPTY_GOAL_FORM;
      adoptedRef.current = id;
      forceAdoptRef.current = false;
      setForm(next);
      setSaved(next);
      return;
    }
    if (active) {
      setForm((f) => withServerDoneStates(f, active, inFlight.current));
      setSaved((f) => withServerDoneStates(f, active, inFlight.current));
    }
  }, [data, active, inFlight]);

  return { form, setForm, saved, setSaved, forceAdoptRef };
}

type Params = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  data: GoalResponse | null;
  reload: () => void;
};

// Owns the goal form, its save-bar contributor, the immediate per-criterion
// toggles and the Mark milestone met request for one coordinator.
export function useGoalEditor({ workspaceId, coordinatorId, canManage, data, reload }: Params) {
  const { t } = useTranslation();
  const active: Goal | null = data?.active ?? null;
  const [fieldError, setFieldError] = useState<GoalFieldError | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(new Set<string>());
  const { form, setForm, saved, setSaved, forceAdoptRef } = useAdoptedGoalForm(data, inFlight);

  const conflict = useCallback(() => {
    forceAdoptRef.current = true;
    setNotice(t("coordinator:goalConflict"));
    reload();
  }, [reload, t]);

  const handleFailure = useCallback(
    (error: unknown) => {
      const validation = fieldErrorOf(error);
      if (error instanceof ApiError && error.status === 409) return conflict();
      if (validation && isStaleCriterionId(validation)) return conflict();
      if (validation) return setFieldError(validation);
      if (error instanceof ApiError && error.status === 403) {
        return setNotice(t("coordinator:goalPermissionError"));
      }
      setNotice(t("coordinator:goalSaveFailed"));
    },
    [conflict, t],
  );

  const send = async (request: () => Promise<Goal>, onDone: (goal: Goal) => void) => {
    setBusy(true);
    setFieldError(null);
    setNotice(null);
    try {
      onDone(await request());
    } catch (error) {
      handleFailure(error);
      throw error;
    } finally {
      setBusy(false);
    }
  };

  const saveGoal = () =>
    send(
      () => putGoal(workspaceId, coordinatorId, buildPutGoalRequest(form, active?.id)),
      (goal) => {
        const next = goalFormFromGoal(goal);
        setForm(next);
        setSaved(next);
        reload();
      },
    );

  const markMet = async () => {
    if (!active) return false;
    try {
      await send(
        () => markGoalMet(workspaceId, coordinatorId, active.id),
        () => {
          forceAdoptRef.current = true;
          reload();
        },
      );
      return true;
    } catch {
      return false;
    }
  };

  const dirty = canManage && active !== null && isGoalFormDirty(form, saved);
  useSettingsSaveContributor({
    id: `coordinator-goal:${coordinatorId}`,
    revision: JSON.stringify(form),
    isDirty: dirty,
    canSave: !busy,
    save: saveGoal,
    discard: () => {
      setForm(saved);
      setFieldError(null);
    },
  });

  const toggles = useCriterionToggles(
    { workspaceId, coordinatorId, reload, inFlight },
    setForm,
    setSaved,
  );

  return {
    active,
    form,
    setForm,
    dirty,
    busy,
    fieldError,
    notice,
    saveGoal,
    markMet,
    toggleCriterion: toggles,
    setFieldError,
  };
}

type ToggleContext = {
  workspaceId: string;
  coordinatorId: string;
  reload: () => void;
  inFlight: React.MutableRefObject<Set<string>>;
};

function useCriterionToggles(
  { workspaceId, coordinatorId, reload, inFlight: running }: ToggleContext,
  setForm: React.Dispatch<React.SetStateAction<GoalFormState>>,
  setSaved: React.Dispatch<React.SetStateAction<GoalFormState>>,
) {
  const { t } = useTranslation();
  const desired = useRef(new Map<string, boolean>());

  const drain = async (criterionId: string) => {
    if (running.current.has(criterionId)) return;
    running.current.add(criterionId);
    try {
      while (desired.current.has(criterionId)) {
        const want = desired.current.get(criterionId) as boolean;
        desired.current.delete(criterionId);
        const goal = await setGoalCriterionDone(workspaceId, coordinatorId, criterionId, want);
        if (desired.current.has(criterionId)) continue;
        setForm((f) => withCriterionDoneState(f, goal, criterionId));
        setSaved((f) => withCriterionDoneState(f, goal, criterionId));
      }
    } catch (error) {
      desired.current.delete(criterionId);
      if (!(error instanceof ApiError && error.status === 404)) {
        toast.error(t("coordinator:goalSaveFailed"));
      }
      reload();
    } finally {
      running.current.delete(criterionId);
    }
  };

  return (criterionId: string, done: boolean) => {
    desired.current.set(criterionId, done);
    setForm((f) => ({
      ...f,
      criteria: f.criteria.map((c) => (c.id === criterionId ? { ...c, done } : c)),
    }));
    void drain(criterionId);
  };
}
