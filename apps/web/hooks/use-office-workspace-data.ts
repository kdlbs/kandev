"use client";

import { useCallback, useEffect, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { getInbox, getMeta, listAgentProfiles, listProjects } from "@/lib/api/domains/office-api";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { AppState } from "@/lib/state/store";
import { selectOfficeProject } from "@/lib/state/slices/office/selectors";
import type { Project } from "@/lib/state/slices/office/types";
import { isOfficeWorkspace, selectActiveWorkspace } from "@/lib/state/slices/workspace/selectors";
import type { StoreApi } from "zustand";

type OfficeStore = StoreApi<AppState>;

const requestSequences = new WeakMap<OfficeStore, Map<string, number>>();
const inFlightProjectLoads = new WeakMap<OfficeStore, Map<string, Promise<void>>>();

function nextRequestSequence(store: OfficeStore, key: string): number {
  let sequences = requestSequences.get(store);
  if (!sequences) {
    sequences = new Map();
    requestSequences.set(store, sequences);
  }
  const next = (sequences.get(key) ?? 0) + 1;
  sequences.set(key, next);
  return next;
}

function isLatestRequest(store: OfficeStore, key: string, sequence: number): boolean {
  return requestSequences.get(store)?.get(key) === sequence;
}

/**
 * Loads one workspace's agents and writes only the newest result for that
 * store and workspace. All Office consumers use this function so page-level
 * refreshes cannot overwrite a newer sidebar request.
 */
export async function loadOfficeAgents(
  store: OfficeStore,
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  const key = `agents:${workspaceId}`;
  const sequence = nextRequestSequence(store, key);
  const response = await listAgentProfiles(workspaceId, options).catch(() => null);
  if (!response || !isLatestRequest(store, key, sequence)) return;
  store.getState().setOfficeAgentProfiles(workspaceId, response.agents ?? []);
}

/** Loads one workspace's projects with shared stale-response protection. */
export function loadOfficeProjects(
  store: OfficeStore,
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  const key = `projects:${workspaceId}`;
  let loads = inFlightProjectLoads.get(store);
  if (!loads) {
    loads = new Map();
    inFlightProjectLoads.set(store, loads);
  }
  const existing = loads.get(key);
  if (existing) return existing;

  const sequence = nextRequestSequence(store, key);
  const request = listProjects(workspaceId, options)
    .then((response) => {
      if (!isLatestRequest(store, key, sequence)) return;
      store.getState().setProjects(workspaceId, response.projects ?? []);
    })
    .catch(() => {})
    .finally(() => {
      if (loads?.get(key) === request) loads.delete(key);
    });
  loads.set(key, request);
  return request;
}

/**
 * One project, read from the store, for surfaces that can boot without the
 * office collections — a cold `/t/:id` hydrates the task but not the
 * workspace's projects, so the ancestry crumb would have nothing to name.
 *
 * A miss triggers `loadOfficeProjects`, the same workspace-level action the
 * office chrome uses. Going through the store rather than fetching into
 * component state is what keeps the two callers on one write path: the record
 * is shared instead of duplicated, later workspace refreshes update this crumb
 * too, and the loader's newest-request rule still decides who wins.
 *
 * One attempt per (workspace, project). A failed load leaves the crumb absent,
 * which is the harmless failure mode; retrying on every render is not.
 */
export function useOfficeProject(projectId: string | null | undefined): Project | undefined {
  const store = useAppStoreApi();
  const officeEnabled = useFeature("office");
  const activeWorkspace = useAppStore(selectActiveWorkspace);
  const project = useAppStore((state) => selectOfficeProject(state, projectId));
  const requestedKey = useRef<string | null>(null);

  const workspaceId =
    activeWorkspace && officeEnabled && isOfficeWorkspace(activeWorkspace)
      ? activeWorkspace.id
      : null;

  useEffect(() => {
    if (!projectId || !workspaceId || project) return;
    const key = `${workspaceId}:${projectId}`;
    if (requestedKey.current === key) return;
    requestedKey.current = key;
    void loadOfficeProjects(store, workspaceId, { cache: "no-store" });
  }, [project, projectId, store, workspaceId]);

  return project;
}

/** Loads one workspace's inbox with shared stale-response protection. */
export async function loadOfficeInbox(
  store: OfficeStore,
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<void> {
  const key = `inbox:${workspaceId}`;
  const sequence = nextRequestSequence(store, key);
  const response = await getInbox(workspaceId, options).catch(() => null);
  if (!response || !isLatestRequest(store, key, sequence)) return;
  const items = response.items ?? [];
  store.getState().setInboxItems(workspaceId, items);
  store.getState().setInboxCount(workspaceId, response.total_count ?? items.length);
}

/** Loads instance-wide Office metadata with the same newest-request rule. */
export async function loadOfficeMeta(
  store: OfficeStore,
  options?: ApiRequestOptions,
): Promise<void> {
  const sequence = nextRequestSequence(store, "meta");
  const response = await getMeta(options).catch(() => null);
  if (!response || !isLatestRequest(store, "meta", sequence)) return;
  store.getState().setMeta(response);
}

/**
 * Loads the office collections the always-mounted chrome reads — agents,
 * projects and the inbox — for the active workspace, plus the global office
 * meta.
 *
 * Owning the fetch in always-mounted chrome — rather than in the office routes
 * — makes the data a property of the selected workspace rather than of the
 * URL, so the sidebar's sections and the inbox badge are filled on every
 * surface, not just under `/office/*`.
 *
 * Deliberately gated on the workspace record, not on `useInOffice()`: the point
 * is that an office workspace has its data loaded wherever you are. The
 * surfaces that render it are still route-gated today, so this only changes
 * *when* the data is available, not what is shown.
 */
export function useOfficeWorkspaceData(): void {
  const store = useAppStoreApi();
  const officeEnabled = useFeature("office");
  const activeWorkspace = useAppStore(selectActiveWorkspace);
  // Office endpoints are not registered when the feature is off, so a kanban
  // workspace (or a build without Office) must not call them at all.
  const workspaceId =
    activeWorkspace && officeEnabled && isOfficeWorkspace(activeWorkspace)
      ? activeWorkspace.id
      : null;

  // A failed refresh is a no-write: this hook refreshes data other surfaces
  // already hydrated, so writing a fallback on rejection would blank the last
  // known-good agents, projects, or inbox badge over a transient error.
  const loadAgents = useCallback(async () => {
    if (!workspaceId) return;
    await loadOfficeAgents(store, workspaceId, { cache: "no-store" });
  }, [store, workspaceId]);

  const loadProjects = useCallback(async () => {
    if (!workspaceId) return;
    await loadOfficeProjects(store, workspaceId, { cache: "no-store" });
  }, [store, workspaceId]);

  const loadInbox = useCallback(async () => {
    if (!workspaceId) return;
    await loadOfficeInbox(store, workspaceId, { cache: "no-store" });
  }, [store, workspaceId]);

  useEffect(() => {
    if (!workspaceId) return;

    async function load() {
      await Promise.all([
        loadAgents(),
        loadProjects(),
        loadInbox(),
        loadOfficeMeta(store, { cache: "no-store" }),
      ]);
    }

    void load();
  }, [loadAgents, loadInbox, loadProjects, store, workspaceId]);

  // The same WS triggers the office pages listen to, so a change made in one
  // surface refreshes the chrome in every other.
  useOfficeRefetch("agents", loadAgents);
  useOfficeRefetch("projects", loadProjects);
  useOfficeRefetch("inbox", loadInbox);
}
