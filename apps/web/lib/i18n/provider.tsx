import { I18nextProvider } from "react-i18next";
import type { ReactNode } from "react";

import { i18n, initI18n } from "./index";
import { resolveInitialLocale } from "./boot";
import { readBootPayload } from "@/src/boot-payload";

/**
 * Initialize before any component can call `useTranslation()`.
 *
 * Deliberately at module load rather than inside the component: the instance
 * has to be ready before React renders anything, because react-i18next suspends
 * on an uninitialized instance and nothing above the root catches that — the
 * tree simply never commits, giving a blank page with no console error, no page
 * error and no unhandled rejection to explain it.
 *
 * This used to happen by accident. `activateLocale()` only runs from the
 * language switcher, so at boot the instance was initialized purely because
 * some module called the module-level `t()` while evaluating. Removing the last
 * such call blanked the entire app. The boot path now owns this explicitly —
 * see docs/i18n.md ("How it's wired").
 */
initI18n(resolveInitialLocale(typeof window === "undefined" ? undefined : readBootPayload()));

/**
 * Binds react-i18next to the shared i18next instance. `changeLanguage` notifies
 * subscribers, so a locale switch re-renders the whole tree without a reload.
 * Mounted as the outermost provider in app-shell so every surface (toasts,
 * dialogs, sidebar) is covered.
 */
export function I18nProvider({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}
