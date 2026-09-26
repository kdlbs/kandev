"use client";

import { useLocalStorageBoolean } from "@/hooks/use-local-storage-boolean";

const STORAGE_KEY = "kandev:integrations:hideDisabledInNav:v1";
const SYNC_EVENT = "kandev:integrations:hide-disabled-in-nav-changed";

/**
 * The integrations index page's "Hide disabled integrations from left panel
 * navigation" setting: a single, install-wide, `localStorage`-backed boolean
 * (not per-integration), defaulting to `false`. Delegates to the shared
 * `useLocalStorageBoolean` primitive (same storage-event + custom-event
 * broadcast shape all nav-visibility toggles use), so the integrations and
 * agent-profiles settings cannot drift in mechanism.
 */
export function useHideDisabledIntegrationsInNav() {
  const { value: hideDisabled, setValue: setHideDisabled } = useLocalStorageBoolean(
    STORAGE_KEY,
    SYNC_EVENT,
  );
  return { hideDisabled, setHideDisabled };
}
