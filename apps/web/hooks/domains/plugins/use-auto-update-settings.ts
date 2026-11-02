"use client";

import { useEffect, useRef, useState } from "react";
import { toast } from "@/lib/toast/sonner";
import { getPluginSettings, updatePluginSettings } from "@/lib/api/domains/plugins-api";
import { t } from "@/lib/i18n";

export type AutoUpdateSettings = {
  /** The instance-wide auto-update default (applies to plugins without an override). */
  autoUpdateDefault: boolean;
  /** False until the initial GET resolves, so the UI can avoid flashing a stale value. */
  loaded: boolean;
  /** Queue an immediate optimistic default; resolve when its PUT settles. */
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
  const accepted = useRef<boolean | undefined>(undefined);
  const latestIntent = useRef(0);
  const writes = useRef(Promise.resolve());
  const mounted = useRef(false);

  useEffect(() => {
    let cancelled = false;
    mounted.current = true;
    getPluginSettings()
      .then((s) => {
        if (!cancelled && latestIntent.current === 0) {
          accepted.current = s.auto_update_default;
          setState(s.auto_update_default);
        }
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
      mounted.current = false;
    };
  }, []);

  const setDefault = async (value: boolean) => {
    if (!mounted.current) return;
    const intent = ++latestIntent.current;
    setState(value); // optimistic
    // Serialize persistence as well as publication; older writes cannot arrive
    // at the server after newer choices from this page.
    writes.current = writes.current.then(async () => {
      try {
        const res = await updatePluginSettings(value);
        accepted.current = res.auto_update_default;
        if (mounted.current && intent === latestIntent.current) setState(accepted.current);
      } catch (err) {
        if (!mounted.current) return;
        if (intent === latestIntent.current) setState(accepted.current ?? false);
        toast.error(err instanceof Error ? err.message : t("plugins:failedToUpdateAutoUpdate"));
      }
    });
    return writes.current;
  };

  return { autoUpdateDefault, loaded, setDefault };
}
