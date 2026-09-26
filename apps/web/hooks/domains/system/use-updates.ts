"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { checkUpdates, fetchUpdates, saveUpdatesChannel } from "@/lib/api/domains/system-api";
import type { UpdatesChannel, UpdatesResponse } from "@/lib/types/system";

type UpdatesRequestCoordinator = {
  readRevision: number;
  saveRevision: number;
  activeSaves: number;
  saveTail: Promise<void>;
  reloadFlight: {
    request: number;
    promise: Promise<UpdatesResponse>;
    published: boolean;
  } | null;
};

// Hooks own their loading/error UI, but every instance writes the same Zustand
// store. Coordinate response authority per store so one mounted reader cannot
// overwrite a newer channel save performed by another instance.
const coordinators = new WeakMap<object, UpdatesRequestCoordinator>();

function coordinatorFor(store: object): UpdatesRequestCoordinator {
  let coordinator = coordinators.get(store);
  if (!coordinator) {
    coordinator = {
      readRevision: 0,
      saveRevision: 0,
      activeSaves: 0,
      saveTail: Promise.resolve(),
      reloadFlight: null,
    };
    coordinators.set(store, coordinator);
  }
  return coordinator;
}

function revalidateWithoutReplacingError(
  coordinator: UpdatesRequestCoordinator,
  setSystemUpdates: (updates: UpdatesResponse) => void,
): void {
  const request = ++coordinator.readRevision;
  void fetchUpdates({ cache: "no-store" })
    .then((response) => {
      if (request === coordinator.readRevision && coordinator.activeSaves === 0) {
        setSystemUpdates(response);
      }
    })
    .catch(() => {
      // Keep the triggering error authoritative; a later manual reload can retry.
    });
}

function queueChannelSave(
  coordinator: UpdatesRequestCoordinator,
  channel: UpdatesChannel,
  setSystemUpdates: (updates: UpdatesResponse) => void,
  setError: (error: string | null) => void,
  saveErrorMessage: string,
): Promise<UpdatesResponse> {
  const request = ++coordinator.saveRevision;
  coordinator.activeSaves += 1;
  coordinator.readRevision += 1;
  setError(null);
  const previousSave = coordinator.saveTail;
  const operation = (async () => {
    let saved = false;
    await previousSave;
    try {
      const response = await saveUpdatesChannel(channel);
      saved = true;
      if (request === coordinator.saveRevision) setSystemUpdates(response);
      return response;
    } catch (error) {
      if (request === coordinator.saveRevision) {
        console.error("[updates] Failed to save update channel", error);
        setError(saveErrorMessage);
      }
      throw error;
    } finally {
      coordinator.activeSaves -= 1;
      coordinator.readRevision += 1;
      if (!saved && request === coordinator.saveRevision && coordinator.activeSaves === 0) {
        revalidateWithoutReplacingError(coordinator, setSystemUpdates);
      }
    }
  })();
  coordinator.saveTail = operation.then(
    () => undefined,
    () => undefined,
  );
  return operation;
}

export function useUpdates() {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const coordinator = coordinatorFor(store);
  const updates = useAppStore((s) => s.system.updates);
  const setSystemUpdates = useAppStore((s) => s.setSystemUpdates);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const latestCheck = useRef(0);

  const reload = useCallback(async () => {
    setError(null);
    let flight = coordinator.reloadFlight;
    if (!flight || flight.request !== coordinator.readRevision) {
      flight = {
        request: ++coordinator.readRevision,
        promise: fetchUpdates({ cache: "no-store" }),
        published: false,
      };
      coordinator.reloadFlight = flight;
    }
    try {
      const res = await flight.promise;
      if (
        !flight.published &&
        flight.request === coordinator.readRevision &&
        coordinator.activeSaves === 0
      ) {
        flight.published = true;
        setSystemUpdates(res);
      }
    } catch (e) {
      if (flight.request === coordinator.readRevision && coordinator.activeSaves === 0) {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      if (coordinator.reloadFlight === flight) coordinator.reloadFlight = null;
    }
  }, [coordinator, setSystemUpdates]);

  /**
   * Triggers a server-side re-poll of the selected update channel. The
   * backend rate-limits this per-process to one call per 30s and replies
   * with the fresh row (or 429 — surfaced via the returned promise).
   */
  const check = useCallback(async () => {
    const request = ++coordinator.readRevision;
    const checkRequest = ++latestCheck.current;
    setIsChecking(true);
    setError(null);
    try {
      const res = await checkUpdates();
      if (request === coordinator.readRevision && coordinator.activeSaves === 0) {
        setSystemUpdates(res);
      }
      return res;
    } catch (e) {
      if (request !== coordinator.readRevision || coordinator.activeSaves > 0) return undefined;
      setError(e instanceof Error ? e.message : String(e));
      if (!store.getState().system.updates) {
        revalidateWithoutReplacingError(coordinator, setSystemUpdates);
      }
      throw e;
    } finally {
      if (checkRequest === latestCheck.current) setIsChecking(false);
    }
  }, [coordinator, setSystemUpdates, store]);

  const saveChannel = useCallback(
    (channel: UpdatesChannel) =>
      queueChannelSave(
        coordinator,
        channel,
        setSystemUpdates,
        setError,
        t("settings:updateChannelSaveFailed"),
      ),
    [coordinator, setSystemUpdates, t],
  );

  useEffect(() => {
    if (updates) return;
    void reload();
  }, [updates, reload]);

  return { updates, isChecking, error, reload, check, saveChannel };
}
