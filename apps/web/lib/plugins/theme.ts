/**
 * Resolved light/dark theme for the plugin host (`host.theme` /
 * `host.onThemeChange`).
 *
 * `AppThemeProvider` (components/theme/app-theme.tsx) applies the resolved
 * theme by toggling the `light`/`dark` class on `<html>`, and emits no event
 * of its own. Observing that class is therefore the only source that covers
 * every way the theme can flip — the settings picker, a live preview hover,
 * and an OS-level `prefers-color-scheme` change while the app sits on
 * "system" — without the plugin host having to subscribe to React context it
 * is mounted outside of.
 */

export type ResolvedTheme = "light" | "dark";

/** Reads the resolved light/dark theme currently applied to the document. */
export function readResolvedTheme(doc: Document = document): ResolvedTheme {
  return doc.documentElement.classList.contains("dark") ? "dark" : "light";
}

type ThemeListener = (theme: ResolvedTheme) => void;

const listeners = new Set<ThemeListener>();
let observer: MutationObserver | null = null;
let lastTheme: ResolvedTheme | null = null;

/**
 * Fans a class mutation out to subscribers, but only when the *resolved*
 * theme actually changed. `<html>`'s class list carries far more than the
 * theme (Tailwind state classes, the rendering-engine marker, transition
 * suppression), so an unfiltered observer would wake every plugin on
 * unrelated churn — expensive for the canvas repaints this exists to drive.
 */
function handleMutation(): void {
  const next = readResolvedTheme();
  if (next === lastTheme) return;
  lastTheme = next;
  for (const listener of [...listeners]) listener(next);
}

/**
 * Subscribes to resolved-theme changes; returns an unsubscribe function.
 *
 * One `MutationObserver` is shared across every subscriber and is connected
 * lazily on the first subscription / disconnected after the last one, so a
 * page with no plugins (or none that care about the theme) observes nothing.
 */
export function subscribeToThemeChanges(listener: ThemeListener): () => void {
  listeners.add(listener);
  if (!observer) {
    lastTheme = readResolvedTheme();
    observer = new MutationObserver(handleMutation);
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && observer) {
      observer.disconnect();
      observer = null;
      lastTheme = null;
    }
  };
}
