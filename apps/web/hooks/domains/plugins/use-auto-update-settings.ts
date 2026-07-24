"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { getPluginSettings, updatePluginSettings } from "@/lib/api/domains/plugins-api";

export type AutoUpdateSettings = {
  /** The instance-wide auto-update default (applies to plugins without an override). */
  autoUpdateDefault: boolean;
  /** False until the initial GET resolves, so the UI can avoid flashing a stale value. */
  loaded: boolean;
  /** Persist a new default (PUT); optimistic, rolling back on failure. */
  setDefault: (value: boolean) => Promise<void>;
};

/**
 * Loads and mutates the instance-wide plugin auto-update default
 * (GET/PUT /api/plugins/settings). The default seeds the effective state of
 * every per-plugin toggle that has no explicit override, so it lives here at
 * the page level and is passed down to each row.
 */
export function useAutoUpdateSettings(): AutoUpdateSettings {
  const [autoUpdateDefault, setState] = useState(false);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getPluginSettings()
      .then((s) => {
        if (!cancelled) setState(s.auto_update_default);
      })
      .catch(() => {
        // A failed read leaves the default at its opt-in-off value; the toggle
        // still works and a later PUT will reconcile.
      })
      .finally(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const setDefault = async (value: boolean) => {
    const prev = autoUpdateDefault;
    setState(value); // optimistic
    try {
      const res = await updatePluginSettings(value);
      setState(res.auto_update_default);
    } catch (err) {
      setState(prev); // roll back
      toast.error(err instanceof Error ? err.message : "Failed to update auto-update setting");
    }
  };

  return { autoUpdateDefault, loaded, setDefault };
}
