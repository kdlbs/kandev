"use client";

import { useTranslation } from "react-i18next";

/**
 * The badges that qualify a record wherever it is listed.
 *
 * Each of these says something the row itself cannot: which workspace commands
 * ended up acting on, and which profiles a picker will refuse to offer. Both
 * facts matter in the settings menu and on the record's own page, so both live
 * here rather than being redrawn per surface — a badge that reads differently
 * in the menu than on the page it opens is worse than no badge.
 */

/** The workspace new tasks and commands act on. */
export function ActiveWorkspaceBadge() {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded-full border border-primary/35 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
      {t("sidebar:activeWorkspaceBadge")}
    </span>
  );
}

/**
 * An agent whose CLI is not on this machine. Its profiles stay listed and
 * editable — you may be about to install it — but nothing can run against it.
 *
 * Neutral rather than amber, though the profile page draws this same fact as an
 * amber panel: in the menu a not-installed agent sits directly above its own
 * profiles, so a second amber pill would read as another "you turned this off"
 * rather than as "this is not here". Yours to fix versus not yours to fix are
 * different kinds of problem and should not share a colour at one glance.
 */
export function NotInstalledBadge() {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded border border-muted-foreground/40 bg-muted px-1 py-0.5 text-[9px] font-medium leading-none text-muted-foreground">
      {t("agents:notInstalled")}
    </span>
  );
}

/**
 * A profile switched off. It is hidden from every task and session picker but
 * stays listed so it can be edited and switched back on — which is exactly why
 * it has to be marked, or it reads as an ordinary profile that pickers have
 * inexplicably lost.
 */
export function DisabledBadge() {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded border border-amber-500/30 bg-amber-500/10 px-1 py-0.5 text-[9px] font-medium leading-none text-amber-600 dark:text-amber-400">
      {t("sidebar:disabledBadge")}
    </span>
  );
}
