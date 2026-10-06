"use client";

import { useEffect, useLayoutEffect, useMemo } from "react";
import type { StoreApi } from "zustand";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { SettingsSaveCancelledError } from "@/components/settings/settings-save-provider";
import {
  fetchSleepInhibitionSettings,
  updateSleepInhibitionSettings,
} from "@/lib/api/domains/settings-api";
import type { AppState } from "@/lib/state/store";
import type { SleepInhibitionResponse, SleepInhibitionSettings } from "@/lib/types/system";

const STATUS_POLL_INTERVAL_MS = 15_000;

type Lifetime = { active: boolean; flight: Read | null };
type Read = { lifetime: Lifetime; generation: number };
type Publication = { generation: number; current: Read | null };
type Activation = {
  refresh: () => Promise<void>;
  save: (settings: SleepInhibitionSettings) => Promise<SleepInhibitionResponse>;
  retire: () => void;
};
const publications = new WeakMap<StoreApi<AppState>, Publication>();
const inactiveActions = {
  refresh: async () => {},
  save: async (): Promise<SleepInhibitionResponse> => {
    throw new SettingsSaveCancelledError();
  },
};

function publicationFor(store: StoreApi<AppState>): Publication {
  let publication = publications.get(store);
  if (!publication) {
    publication = { generation: 0, current: null };
    publications.set(store, publication);
  }
  return publication;
}

function activate(store: StoreApi<AppState>): Activation {
  const publication = publicationFor(store);
  const lifetime: Lifetime = { active: true, flight: null };
  const isCurrent = (read: Read) =>
    lifetime.active &&
    lifetime.flight === read &&
    publication.current === read &&
    publication.generation === read.generation;

  const refresh = async () => {
    if (!lifetime.active || lifetime.flight?.generation === publication.generation) return;
    const read = { lifetime, generation: publication.generation };
    lifetime.flight = read;
    publication.current = read;
    const state = store.getState();
    state.setSleepInhibitionLoading(true);
    state.setSleepInhibitionError(false);
    try {
      const next = await fetchSleepInhibitionSettings();
      if (isCurrent(read)) state.setSleepInhibition(next);
    } catch {
      if (isCurrent(read)) state.setSleepInhibitionError(true);
    } finally {
      if (isCurrent(read)) {
        publication.current = null;
        state.setSleepInhibitionLoading(false);
      }
      if (lifetime.flight === read) lifetime.flight = null;
    }
  };

  const save = async (settings: SleepInhibitionSettings) => {
    if (!lifetime.active) throw new SettingsSaveCancelledError();
    const next = await updateSleepInhibitionSettings(settings);
    // An admitted write belongs to its captured store even after the view leaves.
    publication.generation += 1;
    publication.current = null;
    store.getState().setSleepInhibition(next);
    return next;
  };

  const retire = () => {
    lifetime.active = false;
    lifetime.flight = null;
    if (publication.current?.lifetime === lifetime) {
      publication.current = null;
      store.getState().setSleepInhibitionLoading(false);
    }
  };
  return { refresh, save, retire };
}

/** Loads the install-wide preference and refreshes host status while settings are open. */
export function useSleepInhibitionSettings() {
  const store = useAppStoreApi();
  const response = useAppStore((state) => state.sleepInhibition.response);
  const loaded = useAppStore((state) => state.sleepInhibition.loaded);
  const loading = useAppStore((state) => state.sleepInhibition.loading);
  const error = useAppStore((state) => state.sleepInhibition.error);
  const binding = useMemo(() => ({ current: null as Activation | null }), [store]);

  useLayoutEffect(() => {
    const activation = activate(store);
    binding.current = activation;
    return activation.retire;
  }, [binding, store]);

  useEffect(() => {
    if (!loaded) void binding.current?.refresh();
  }, [binding, loaded]);

  useEffect(() => {
    if (!loaded) return;
    const activation = binding.current;
    const poll = () => {
      if (document.visibilityState === "visible") void activation?.refresh();
    };
    const interval = window.setInterval(poll, STATUS_POLL_INTERVAL_MS);
    document.addEventListener("visibilitychange", poll);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", poll);
    };
  }, [binding, loaded]);

  return {
    response,
    loaded,
    loading,
    error,
    // Each retrieved callback captures its producing committed activation.
    get refresh() {
      return binding.current?.refresh ?? inactiveActions.refresh;
    },
    get save() {
      return binding.current?.save ?? inactiveActions.save;
    },
  };
}
