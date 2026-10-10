"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import { mapLatestUserSettingsResponse } from "@/lib/ssr/user-settings";
import { fromApiSidebarLayout, toApiSidebarLayout } from "@/lib/sidebar/layout-types";
import { materializeSidebarPluginNodes } from "@/lib/sidebar/layout-projection";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { createSidebarWriter, type SidebarMutation } from "@/lib/sidebar/immediate-writer";
import type {
  UserSettingsResponse,
  UserSettingsUpdatePayload,
} from "@/lib/types/http-user-settings";

type Store = ReturnType<typeof useAppStoreApi>;
const writers = new WeakMap<Store, ReturnType<typeof createSidebarWriter>>();
const preferenceQueues = new WeakMap<Store, Promise<void>>();

function acceptSettings(store: Store, response: UserSettingsResponse) {
  const current = store.getState().userSettings;
  store.getState().setUserSettings(mapLatestUserSettingsResponse(response, current));
}

async function saveSettings(store: Store, patch: UserSettingsUpdatePayload) {
  try {
    acceptSettings(store, await updateUserSettings(patch));
  } catch (error) {
    try {
      acceptSettings(store, await fetchUserSettings({ cache: "no-store" }));
    } catch {
      // A failed refresh must preserve the original save error.
    }
    throw error;
  }
}

function writerFor(store: Store) {
  let writer = writers.get(store);
  if (!writer) {
    writer = createSidebarWriter({
      read: (workspaceId) =>
        fromApiSidebarLayout(store.getState().userSettings.sidebarLayoutsByWorkspace[workspaceId]),
      write: async (workspaceId, layout) => {
        if (layout.unsupportedVersion)
          throw new ApiError("settings:sidebarLayoutInvalidError", 400, null);
        await saveSettings(store, {
          sidebar_layout_state: {
            workspace_id: workspaceId,
            expected_revision: layout.revision,
            layout: toApiSidebarLayout(layout),
          },
        });
      },
    });
    writers.set(store, writer);
  }
  return writer;
}

export function useSidebarCustomization(catalog: ShortcutCatalogEntry[] = []) {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const unsupported = useAppStore((s) =>
    Boolean(
      s.workspaces.activeId &&
      fromApiSidebarLayout(s.userSettings.sidebarLayoutsByWorkspace[s.workspaces.activeId])
        .unsupportedVersion,
    ),
  );
  const [status, setStatus] = useState<"saving" | "error" | "conflict" | null>(null);
  const scope = useRef(workspaceId);
  scope.current = workspaceId;
  useEffect(() => setStatus(null), [workspaceId]);
  const run = useCallback(
    async (job: () => Promise<void>) => {
      const requestedScope = workspaceId;
      setStatus("saving");
      try {
        await job();
        if (scope.current === requestedScope) setStatus(null);
      } catch (error) {
        if (scope.current === requestedScope)
          setStatus(error instanceof ApiError && error.status === 409 ? "conflict" : "error");
      }
    },
    [workspaceId],
  );
  const mutate = useCallback(
    (operation: SidebarMutation) => {
      if (!workspaceId || unsupported) return Promise.resolve();
      return run(() =>
        writerFor(store)(workspaceId, (current) =>
          operation(materializeSidebarPluginNodes(current, catalog)),
        ),
      );
    },
    [catalog, run, store, unsupported, workspaceId],
  );
  const preferences = useCallback(
    (patch: UserSettingsUpdatePayload) =>
      run(() => {
        const job = (preferenceQueues.get(store) ?? Promise.resolve())
          .catch(() => undefined)
          .then(async () => {
            await saveSettings(store, patch);
          });
        preferenceQueues.set(store, job);
        return job;
      }),
    [run, store],
  );
  return { mutate, preferences, status, enabled: Boolean(workspaceId) && !unsupported };
}
