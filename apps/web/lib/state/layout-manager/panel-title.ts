import { t } from "@/lib/i18n";

import { PANEL_REGISTRY } from "./constants";

/**
 * The display title for a registry panel, in the active locale.
 *
 * Deliberately NOT in `constants.ts`. That module is imported by e2e specs
 * (`e2e/tests/layout/pane-resize-right.spec.ts` takes `TERMINAL_DEFAULT_ID` from
 * it), and Playwright loads spec files through Node rather than Vite. Pulling
 * `@/lib/i18n` into that graph makes the whole file fail to load with
 * `SyntaxError: Cannot use 'import.meta' outside a module` — and one unloadable
 * spec aborts the entire run, so it takes every shard down with it, not just
 * the layout tests. Keeping the i18n dependency in a separate module leaves
 * `constants.ts` importable from plain Node.
 *
 * Resolved at call time, never at module load — the registry stores the key.
 * Unknown ids (file diffs, plugin panels, per-session chat tabs) carry their own
 * title and are returned unchanged.
 *
 * This is the RENDER direction only. `LayoutState` — what gets saved — always
 * holds the canonical English title, so the localized form exists solely inside
 * dockview. `toSerializedDockview` localizes on the way in and
 * `panelFromDockviewPanel` canonicalizes on the way back out.
 */
export function panelTitle(id: string, fallback?: string): string {
  const config = PANEL_REGISTRY[id];
  if (!config) return fallback ?? id;
  return config.titleKey ? t(config.titleKey) : config.title;
}
