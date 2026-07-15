"use client";

import { useEffect, useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { createDesktopV1Adapter, type DesktopV1Adapter } from "@/lib/desktop/adapter";
import {
  createDesktopCommandActions,
  subscribeDesktopCommandActions,
} from "@/lib/desktop/command-actions";
import { closeDesktopContext } from "@/lib/desktop/contextual-close";
import {
  desktopExternalLinks,
  subscribeDesktopExternalLinks,
  type DesktopExternalLinkAdapter,
} from "@/lib/desktop/external-links";
import { requestNewTaskCreation } from "@/lib/desktop/new-task-request";
import { createTauriEventTransport } from "@/lib/desktop/tauri-event-transport";
import { desktopUpdater } from "@/lib/desktop/updater-client";
import type { DesktopUpdaterAdapter } from "@/lib/desktop/updater-adapter";

const desktopAdapter = createDesktopV1Adapter(createTauriEventTransport());

export function DesktopCommandHost({
  adapter = desktopAdapter,
  externalLinks = desktopExternalLinks,
  updater = desktopUpdater,
}: {
  adapter?: DesktopV1Adapter;
  externalLinks?: DesktopExternalLinkAdapter;
  updater?: DesktopUpdaterAdapter;
}) {
  const router = useRouter();
  const actions = useMemo(
    () =>
      createDesktopCommandActions({
        closeContext: () => closeDesktopContext(document, useDockviewStore.getState().api),
        navigate: router.push,
        requestNewTask: requestNewTaskCreation,
      }),
    [router.push],
  );

  useEffect(() => {
    if (!adapter.isAvailable()) return;
    let disposed = false;
    let stop: (() => void) | undefined;
    void subscribeDesktopCommandActions(adapter, actions).then(
      (unlisten) => {
        if (disposed) unlisten();
        else stop = unlisten;
      },
      () => undefined,
    );
    return () => {
      disposed = true;
      stop?.();
    };
  }, [actions, adapter]);

  useEffect(() => {
    if (!adapter.isAvailable() || !updater.isAvailable()) return;
    let disposed = false;
    let stop: (() => void) | undefined;
    void adapter
      .listen("check-for-updates", () => {
        router.push("/settings/system/updates");
        void updater.checkForUpdates().catch(() => undefined);
      })
      .then(
        (unlisten) => {
          if (disposed) unlisten();
          else stop = unlisten;
        },
        () => undefined,
      );
    return () => {
      disposed = true;
      stop?.();
    };
  }, [adapter, router, updater]);

  useEffect(() => subscribeDesktopExternalLinks(document, externalLinks), [externalLinks]);

  return null;
}
