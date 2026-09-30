"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  getCoordinator,
  patchCoordinator,
  type PatchCoordinatorRequest,
} from "@/lib/api/domains/coordinator-api";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { ceilingCents, normalizeCeiling } from "@/lib/coordinator/autonomy";
import { toast } from "@/lib/toast/sonner";

export type AutonomyDraft = { enabled: boolean; ceiling: string };
export type AutonomyLoadStatus = "loading" | "ready" | "error";
export type CeilingHint = "invalid" | "required" | "rejected" | null;

type Params = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  refreshAutonomy: () => void;
};

function draftOf(record: { autonomy_enabled?: boolean; cost_ceiling_usd?: string | null }) {
  return { enabled: record.autonomy_enabled === true, ceiling: record.cost_ceiling_usd ?? "" };
}

function ceilingChanged(draft: AutonomyDraft, base: AutonomyDraft): boolean {
  const next = draft.ceiling.trim() === "" ? null : ceilingCents(draft.ceiling);
  const prev = base.ceiling.trim() === "" ? null : ceilingCents(base.ceiling);
  if (draft.ceiling.trim() !== "" && next === null) return true;
  return next !== prev;
}

function hintOf(draft: AutonomyDraft): "invalid" | "required" | null {
  if (draft.ceiling.trim() !== "" && ceilingCents(draft.ceiling) === null) return "invalid";
  if (draft.enabled && draft.ceiling.trim() === "") return "required";
  return null;
}

/** The request carrying only the members that changed; ceilings compare as integer cents. */
export function buildAutonomyPatch(
  draft: AutonomyDraft,
  base: AutonomyDraft,
): PatchCoordinatorRequest {
  const request: PatchCoordinatorRequest = {};
  if (draft.enabled !== base.enabled) request.autonomy_enabled = draft.enabled;
  if (ceilingChanged(draft, base)) {
    request.cost_ceiling_usd = draft.ceiling.trim() === "" ? null : normalizeCeiling(draft.ceiling);
  }
  return request;
}

function namesCeiling(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 400) return false;
  const field = (error.body as { field?: unknown } | null)?.field;
  return typeof field === "string" && field.includes("cost_ceiling");
}

function useAutonomyBaseline(workspaceId: string, coordinatorId: string) {
  const [base, setBase] = useState<AutonomyDraft | null>(null);
  const [draft, setDraft] = useState<AutonomyDraft | null>(null);
  const [status, setStatus] = useState<AutonomyLoadStatus>("loading");
  const baseRef = useRef(base);
  baseRef.current = base;
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const sequenceRef = useRef(0);

  const adopt = useCallback((next: AutonomyDraft, previous: AutonomyDraft | null) => {
    baseRef.current = next;
    setBase(next);
    const current = draftRef.current;
    if (!current || !previous || !isDirty(current, previous)) {
      draftRef.current = next;
      setDraft(next);
    }
  }, []);

  const load = useCallback(async (): Promise<void> => {
    const sequence = ++sequenceRef.current;
    try {
      const record = await getCoordinator(workspaceId, coordinatorId);
      if (sequence !== sequenceRef.current) return;
      adopt(draftOf(record), baseRef.current);
      setStatus("ready");
    } catch {
      if (sequence !== sequenceRef.current) return;
      if (!baseRef.current) setStatus("error");
    }
  }, [workspaceId, coordinatorId, adopt]);

  useEffect(() => {
    baseRef.current = null;
    draftRef.current = null;
    setBase(null);
    setDraft(null);
    setStatus("loading");
    void load();
    return () => {
      sequenceRef.current += 1;
    };
  }, [load]);

  const retry = useCallback(() => {
    setStatus("loading");
    void load();
  }, [load]);

  return { base, draft, status, baseRef, draftRef, sequenceRef, setBase, setDraft, load, retry };
}

const INVALID_REASON: Record<"invalid" | "required", string> = {
  invalid: "coordinator:autonomyCeilingInvalid",
  required: "coordinator:autonomyCeilingRequired",
};

/**
 * The Autonomy section's draft: the toggle and the ceiling saved as one PATCH
 * through one save contributor. The baseline is the coordinator GET.
 */
export function useAutonomySettings({
  workspaceId,
  coordinatorId,
  canManage,
  refreshAutonomy,
}: Params) {
  const { t } = useTranslation();
  const { base, draft, status, baseRef, draftRef, sequenceRef, setBase, setDraft, load, retry } =
    useAutonomyBaseline(workspaceId, coordinatorId);
  const [rejected, setRejected] = useState(false);

  useEffect(() => {
    setRejected(false);
  }, [coordinatorId]);

  const edit = useCallback(
    (change: Partial<AutonomyDraft>) => {
      const current = draftRef.current;
      if (!current) return;
      const next = { ...current, ...change };
      draftRef.current = next;
      setDraft(next);
      setRejected(false);
    },
    [draftRef, setDraft],
  );

  const save = async () => {
    const sent = draftRef.current;
    const before = baseRef.current;
    if (!sent || !before) return;
    const request = buildAutonomyPatch(sent, before);
    if (Object.keys(request).length === 0) return;
    setRejected(false);
    sequenceRef.current += 1;
    try {
      const updated = await patchCoordinator(workspaceId, coordinatorId, request);
      const saved = draftOf(updated);
      baseRef.current = saved;
      setBase(saved);
      const current = draftRef.current ?? sent;
      const settled = current.enabled === sent.enabled && current.ceiling === sent.ceiling;
      if (settled) {
        draftRef.current = saved;
        setDraft(saved);
      }
    } catch (error) {
      if (namesCeiling(error)) setRejected(true);
      else toast.error(t("coordinator:failedToSaveCoordinator"));
      throw error;
    }
    // Refresh reads: a failure of either never turns the successful Save into a failure.
    await load().catch(() => undefined);
    refreshAutonomy();
  };

  const dirty = !!draft && !!base && isDirty(draft, base);
  const blockedHint = draft ? hintOf(draft) : null;
  const hint: CeilingHint = rejected ? "rejected" : blockedHint;
  const blocked = blockedHint !== null;

  useSettingsSaveContributor({
    id: `coordinator-autonomy:${coordinatorId}`,
    revision: JSON.stringify(draft),
    isDirty: canManage && dirty,
    canSave: !blocked,
    invalidReason: blockedHint ? t(INVALID_REASON[blockedHint]) : undefined,
    save,
    discard: () => {
      draftRef.current = baseRef.current;
      setDraft(baseRef.current);
      setRejected(false);
    },
  });

  return { draft, status, hint, edit, retry, dirty };
}

function isDirty(draft: AutonomyDraft, base: AutonomyDraft): boolean {
  return draft.enabled !== base.enabled || ceilingChanged(draft, base);
}
