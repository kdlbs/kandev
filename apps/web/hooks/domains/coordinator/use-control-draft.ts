"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  getCoordinatorSettings,
  putCoordinatorSettings,
  type ControlAction,
  type ControlSetting,
} from "@/lib/api/domains/coordinator-api";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { toast } from "@/lib/toast/sonner";
import { useWebSocketClient } from "@/lib/ws/connection";
import {
  buildPutRequest,
  draftFromSettings,
  isPolicyDirty,
  isWatchesDirty,
  isWatchesInvalid,
  mergeStored,
  settleAfterSave,
  type ControlDraft,
  type WatchesDraft,
} from "@/lib/coordinators/control-draft";

export type ControlLoadStatus = "loading" | "ready" | "error";

/** A 400 or a 409 not_eligible from the settings PUT: the field it names and the closed error code. */
export type ControlFieldError = { field: string | null; code: string | null; detail: string };

function fieldErrorOf(error: unknown): ControlFieldError | null {
  if (!(error instanceof ApiError) || (error.status !== 400 && error.status !== 409)) return null;
  const body = error.body as { error?: unknown; field?: unknown; code?: unknown } | null;
  return {
    field: typeof body?.field === "string" ? body.field : null,
    code: typeof body?.code === "string" ? body.code : null,
    detail: typeof body?.error === "string" ? body.error : error.message,
  };
}

function useControlRead(workspaceId: string, coordinatorId: string) {
  const [stored, setStored] = useState<ControlDraft | null>(null);
  const [draft, setDraft] = useState<ControlDraft | null>(null);
  const [status, setStatus] = useState<ControlLoadStatus>("loading");
  const [fieldError, setFieldError] = useState<ControlFieldError | null>(null);
  const storedRef = useRef(stored);
  storedRef.current = stored;
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const sequenceRef = useRef(0);

  const adopt = useCallback((next: ControlDraft) => {
    const oldStored = storedRef.current;
    const current = draftRef.current;
    storedRef.current = next;
    setStored(next);
    if (!oldStored || !current) {
      draftRef.current = next;
      setDraft(next);
      return;
    }
    const merged = mergeStored(current, oldStored, next);
    draftRef.current = merged;
    setDraft(merged);
  }, []);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    getCoordinatorSettings(workspaceId, coordinatorId)
      .then((settings) => {
        if (sequence !== sequenceRef.current) return;
        adopt(draftFromSettings(settings));
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || storedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, adopt]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  useEffect(() => {
    storedRef.current = null;
    draftRef.current = null;
    setStored(null);
    setDraft(null);
    setFieldError(null);
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [reload]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      reload();
    });
  }, [wsClient, workspaceId, coordinatorId, reload]);

  return {
    stored,
    draft,
    status,
    retry,
    fieldError,
    setFieldError,
    storedRef,
    draftRef,
    sequenceRef,
    setStored,
    setDraft,
  };
}

type Params = { workspaceId: string; coordinatorId: string; canManage: boolean };

/**
 * The one draft behind May do and Watches: one read, one save contributor
 * (`coordinator-control`) and one PUT carrying only the members that changed.
 * A re-read never overwrites an edit, and a PUT invalidates any read sent
 * before it so a stale response cannot undo a save.
 */
export function useControlDraft({ workspaceId, coordinatorId, canManage }: Params) {
  const { t } = useTranslation();
  const read = useControlRead(workspaceId, coordinatorId);
  const {
    stored,
    draft,
    status,
    retry,
    fieldError,
    setFieldError,
    storedRef,
    draftRef,
    sequenceRef,
    setStored,
    setDraft,
  } = read;

  const update = useCallback((change: (current: ControlDraft) => ControlDraft) => {
    const current = draftRef.current;
    if (!current) return;
    const next = change(current);
    draftRef.current = next;
    setDraft(next);
    setFieldError(null);
  }, []);

  const setAction = useCallback(
    (action: ControlAction, value: ControlSetting) =>
      update((d) => ({ ...d, actions: { ...d.actions, [action]: value } })),
    [update],
  );
  const setWatches = useCallback(
    (watches: WatchesDraft) => update((d) => ({ ...d, watches })),
    [update],
  );

  const save = async () => {
    const sent = draftRef.current;
    const base = storedRef.current;
    if (!sent || !base) return;
    const request = buildPutRequest(sent, base);
    if (!request.policy && !request.watches) return;
    setFieldError(null);
    sequenceRef.current += 1;
    try {
      const response = draftFromSettings(
        await putCoordinatorSettings(workspaceId, coordinatorId, request),
      );
      sequenceRef.current += 1;
      storedRef.current = response;
      setStored(response);
      const settled = settleAfterSave(draftRef.current ?? sent, sent, response);
      draftRef.current = settled;
      setDraft(settled);
    } catch (error) {
      const validation = fieldErrorOf(error);
      if (validation) setFieldError(validation);
      else toast.error(t("coordinator:failedToSaveCoordinator"));
      throw error;
    }
  };

  const policyDirty = !!draft && !!stored && isPolicyDirty(draft, stored);
  const watchesDirty = !!draft && !!stored && isWatchesDirty(draft, stored);
  const invalid = !!draft && !!stored && isWatchesInvalid(draft, stored);

  useSettingsSaveContributor({
    id: "coordinator-control",
    revision: JSON.stringify(draft),
    isDirty: canManage && (policyDirty || watchesDirty),
    canSave: !invalid,
    invalidReason: invalid ? t("coordinator:watchesKeepOneBoard") : undefined,
    save,
    discard: () => {
      setDraft(storedRef.current);
      draftRef.current = storedRef.current;
      setFieldError(null);
    },
  });

  return {
    stored,
    draft,
    status,
    retry,
    fieldError,
    setAction,
    setWatches,
    policyDirty,
    watchesDirty,
    invalid,
  };
}
