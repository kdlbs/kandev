"use client";

import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
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
import { useWorkspaceProjects } from "@/hooks/domains/coordinator/use-workspace-projects";
import {
  buildPutRequest,
  draftFromSettings,
  isPolicyDirty,
  isProjectsDirty,
  isProjectsInvalid,
  isWatchesDirty,
  isWatchesInvalid,
  mergeStored,
  settleAfterSave,
  type ControlDraft,
  type ProjectsDraft,
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

function invalidReason(
  t: (key: string) => string,
  watchesInvalid: boolean,
  projectsInvalid: boolean,
): string | undefined {
  if (watchesInvalid) return t("coordinator:watchesKeepOneBoard");
  if (projectsInvalid) return t("coordinator:watchesKeepOneProject");
  return undefined;
}

/**
 * The workspace's project listing, re-read after a rejected foreign entry; once
 * the re-read lands the draft keeps only entries that still exist, and the field
 * error stays visible.
 */
function useProjectsListing(
  workspaceId: string,
  draft: ControlDraft | null,
  draftRef: MutableRefObject<ControlDraft | null>,
  setDraft: (next: ControlDraft) => void,
) {
  const projectsRead = useWorkspaceProjects(workspaceId, draft?.projects != null);
  const pruneRef = useRef(false);
  const { sets, loose, status, retry } = projectsRead;
  useEffect(() => {
    if (!pruneRef.current || status !== "ready") return;
    pruneRef.current = false;
    const current = draftRef.current;
    if (!current?.projects) return;
    const live = new Set([...sets, ...loose].map((c) => `${c.kind}:${c.id}`));
    const entries = current.projects.entries.filter((e) => live.has(`${e.kind}:${e.id}`));
    if (entries.length === current.projects.entries.length) return;
    const next = { ...current, projects: { ...current.projects, entries } };
    draftRef.current = next;
    setDraft(next);
  }, [status, sets, loose, draftRef, setDraft]);
  const pruneAfterRefresh = useCallback(() => {
    pruneRef.current = true;
    retry();
  }, [retry]);
  return { projectsRead, pruneAfterRefresh };
}

function draftFlags(draft: ControlDraft | null, stored: ControlDraft | null) {
  const both = !!draft && !!stored;
  const watchesInvalid = both && isWatchesInvalid(draft, stored);
  const projectsInvalid = both && isProjectsInvalid(draft, stored);
  return {
    policyDirty: both && isPolicyDirty(draft, stored),
    watchesDirty: both && isWatchesDirty(draft, stored),
    projectsDirty: both && isProjectsDirty(draft, stored),
    watchesInvalid,
    projectsInvalid,
    invalid: watchesInvalid || projectsInvalid,
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
  } = useControlRead(workspaceId, coordinatorId);

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
  const setProjects = useCallback(
    (projects: ProjectsDraft) => update((d) => ({ ...d, projects })),
    [update],
  );
  const { projectsRead, pruneAfterRefresh } = useProjectsListing(
    workspaceId,
    draft,
    draftRef,
    setDraft,
  );

  const save = async () => {
    const sent = draftRef.current;
    const base = storedRef.current;
    if (!sent || !base) return;
    const request = buildPutRequest(sent, base);
    if (!request.policy && !request.watches && !request.projects) return;
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
      if (validation?.code === "projects_foreign_entry") pruneAfterRefresh();
      throw error;
    }
  };

  const { policyDirty, watchesDirty, projectsDirty, watchesInvalid, projectsInvalid, invalid } =
    draftFlags(draft, stored);

  useSettingsSaveContributor({
    id: "coordinator-control",
    revision: JSON.stringify(draft),
    isDirty: canManage && (policyDirty || watchesDirty || projectsDirty),
    canSave:
      !invalid && !(draft?.projects?.scope === "selected" && projectsRead.status !== "ready"),
    invalidReason: invalidReason(t, watchesInvalid, projectsInvalid),
    save,
    discard: () => {
      draftRef.current = storedRef.current;
      setDraft(storedRef.current);
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
    setProjects,
    policyDirty,
    watchesDirty,
    projectsDirty,
    invalid,
    projectsRead,
  };
}
