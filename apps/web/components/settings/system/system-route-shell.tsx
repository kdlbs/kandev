"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { SystemPageShell } from "./system-page-shell";

/**
 * The SQL statement that produces a snapshot, and the directory it writes to.
 * Both are interpolated into the Backups description as values: baked into the
 * message, the pseudo-locale renders them `VÀĆŨŨḾ ĨŃŢŌ` and
 * `<ďàţà-ďĩŕ>/ƀàćķũƥś/`, turning a command the user runs and a path they have to
 * find into dead pointers.
 */
export const BACKUP_SQL_COMMAND = "VACUUM INTO";
export const BACKUP_DIR = "<data-dir>/backups/";

/**
 * The shell the System routes render through.
 *
 * Titles and descriptions travel as catalog keys rather than resolved strings
 * because the SPA's route table (`SETTINGS_ROUTES` in `src/settings-routes.tsx`)
 * is a SCREAMING_CASE identifier, which `i18next/no-literal-string` skips
 * entirely — the literals this replaced were invisible to lint and to the
 * new-code ratchet alike.
 */
export function SystemRouteShell({
  titleKey,
  descriptionKey,
  descriptionValues,
  children,
}: {
  titleKey: string;
  descriptionKey: string;
  /** Identifiers the description interpolates, so they survive translation. */
  descriptionValues?: Record<string, string>;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <SystemPageShell title={t(titleKey)} description={t(descriptionKey, descriptionValues)}>
      {children}
    </SystemPageShell>
  );
}
