import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { BACKUP_DIR, BACKUP_SQL_COMMAND } from "./system-route-shell";

/**
 * Byte-for-byte English for the nine System route headers, as `SETTINGS_ROUTES`
 * in `src/settings-routes.tsx` rendered them before this migration.
 *
 * This exists because the copy was duplicated. Each of these routes had an
 * unreferenced `app/settings/system/<route>/page.tsx` twin, and two of the
 * twins had already drifted from the live route table — `logs` was the worse
 * one, because the dead page rendered `settings:logsPageDescription`
 * ("Download a bounded diagnostic ZIP containing frontend and backend logs.")
 * while the live route rendered "Create a diagnostic ZIP with frontend and
 * backend logs.". Pointing the live route at the existing key silently changed
 * user-visible English; only `logs-page.spec.ts` pins that sentence, so the
 * other eight routes had no check at all.
 *
 * An i18n migration must not change copy. This table is the check that says so
 * for all nine, not just the one route that happened to have an e2e assertion.
 */
const ROUTE_COPY: Array<{ route: string; titleKey: string; title: string; description: string }> = [
  {
    route: "status",
    titleKey: "common:status",
    title: "Status",
    description: "Health checks, disk usage, and version summary.",
  },
  {
    route: "feature-toggles",
    titleKey: "system:navFeatureToggles",
    title: "Feature Toggles",
    description: "Manage Kandev feature and diagnostic switches.",
  },
  {
    route: "database",
    titleKey: "system:navDatabase",
    title: "Database",
    description: "Database driver, size, and available maintenance controls.",
  },
  {
    route: "logs",
    titleKey: "settings:logsPageTitle",
    title: "Logs",
    description: "Create a diagnostic ZIP with frontend and backend logs.",
  },
  {
    route: "updates",
    titleKey: "system:navUpdates",
    title: "Updates",
    description: "Current vs latest release plus the full kandev changelog.",
  },
  {
    route: "about",
    titleKey: "system:navAbout",
    title: "About",
    description: "Version, build metadata, and links.",
  },
  {
    route: "licenses",
    titleKey: "system:navLicenses",
    title: "Licenses",
    description: "Open-source licenses for every npm and Go dependency shipped with kandev.",
  },
  {
    route: "users",
    titleKey: "system:navUsers",
    title: "Users",
    description: "Manage accounts, roles, and invite links for this instance.",
  },
];

describe("System route headers keep their pre-migration English", () => {
  it.each(ROUTE_COPY)("$route", ({ titleKey, title, description }) => {
    expect(t(titleKey)).toBe(title);
    const descriptionKey =
      titleKey === "settings:logsPageTitle"
        ? "settings:logsPageDescription"
        : `system:${camelRoute(title)}PageDescription`;
    expect(t(descriptionKey)).toBe(description);
  });

  /**
   * Backups is separate: its description interpolates the SQL statement and the
   * snapshot directory, so both survive translation as values rather than being
   * pseudo-localized into dead pointers.
   */
  it("backups", () => {
    expect(t("system:navBackups")).toBe("Backups");
    expect(
      t("system:backupsPageDescription", { command: BACKUP_SQL_COMMAND, path: BACKUP_DIR }),
    ).toBe("VACUUM INTO snapshots stored under <data-dir>/backups/.");
  });
});

/** "Feature Toggles" -> "featureToggles", matching the catalog key suffix. */
function camelRoute(title: string): string {
  const [first, ...rest] = title.split(" ");
  return first.toLowerCase() + rest.join("");
}
