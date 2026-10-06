"use client";

import { useEffect, useRef } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { useNavContext } from "@/hooks/use-app-destinations";
import { useRouter } from "@/lib/routing/client-router";
import { resolveHref } from "@/lib/navigation/resolve-destinations";
import { coreShortcutEntries, resolveShortcutEntry } from "@/lib/keyboard/plugin-shortcuts";
import { isValidIntegrationShortcut } from "@/lib/keyboard/integration-shortcuts";
import { NON_CONFIGURABLE_CORE_SHORTCUT_IDS } from "@/lib/keyboard/core-shortcuts";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { isEditableKeydownTarget, matchesShortcut } from "@/lib/keyboard/utils";
import { useIntegrationShortcutEntries } from "./use-integration-shortcut-entries";

export function useIntegrationShortcuts(): void {
  const store = useAppStoreApi();
  const entries = useIntegrationShortcutEntries();
  const context = useNavContext();
  const router = useRouter();
  const latest = useRef({ entries, context, router });
  latest.current = { entries, context, router };

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat || isEditableKeydownTarget(event)) return;
      if (document.querySelector('[data-shortcut-recording="true"]')) return;
      const overrides = store.getState().userSettings.keyboardShortcuts;
      const core = coreShortcutEntries().map((entry) => resolveShortcutEntry(entry, overrides));
      const reserved = NON_CONFIGURABLE_CORE_SHORTCUT_IDS.map((id) => SHORTCUTS[id]);
      if ([...core, ...reserved].some((shortcut) => matchesShortcut(event, shortcut))) return;
      const {
        entries: currentEntries,
        context: currentContext,
        router: currentRouter,
      } = latest.current;
      for (const entry of currentEntries) {
        const shortcut = resolveShortcutEntry(entry, overrides);
        if (!isValidIntegrationShortcut(shortcut) || !matchesShortcut(event, shortcut)) continue;
        event.preventDefault();
        event.stopPropagation();
        currentRouter.push(resolveHref(entry.href, currentContext));
        return;
      }
    };
    // Keep registration order stable across context and catalog updates.
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [store]);
}
