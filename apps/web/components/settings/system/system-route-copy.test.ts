import { cleanup, render, screen } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { t } from "@/lib/i18n";
import { DataStorageSettings } from "./data-storage-settings";
import { BACKUP_SQL_COMMAND } from "./system-route-shell";

const databaseState = vi.hoisted(() => ({ value: null as unknown }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { system: { database: unknown } }) => unknown) =>
    selector({ system: { database: databaseState.value } }),
}));
vi.mock("@/components/settings/settings-target", () => ({
  SettingsTarget: ({ children }: { children?: ReactNode }) => children ?? null,
}));
vi.mock("./backups-table", () => ({ BackupsTable: () => null }));
vi.mock("./database-stats-card", () => ({ DatabaseStatsCard: () => null }));
vi.mock("./log-viewer", () => ({ LogViewer: () => null }));
vi.mock("./storage/storage-maintenance-settings", () => ({
  StorageMaintenanceSettings: () => null,
}));

afterEach(() => {
  cleanup();
  databaseState.value = null;
});

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
    // /settings/system/logs now redirects into Data & Storage, whose Logs
    // section titles itself with `system:navLogs`. The description key is
    // unchanged, so the sentence users read is still pinned below.
    route: "logs",
    titleKey: "system:navLogs",
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
      titleKey === "system:navLogs"
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
    const path = "/var/lib/kandev/backups";
    expect(t("system:navBackups")).toBe("Backups");
    expect(t("system:backupsPageDescription", { command: BACKUP_SQL_COMMAND, path })).toBe(
      `VACUUM INTO snapshots stored under ${path}.`,
    );
  });
});

describe("Data & Storage backup location copy", () => {
  it("renders the resolved SQLite backup directory", () => {
    const path = "/var/lib/kandev/backups";
    databaseState.value = { backup_directory: path };

    render(createElement(DataStorageSettings));

    expect(screen.getByText(`VACUUM INTO snapshots stored under ${path}.`)).toBeTruthy();
  });

  it("omits the location when database information is unavailable", () => {
    render(createElement(DataStorageSettings));

    expect(screen.queryByText(/VACUUM INTO snapshots stored under/)).toBeNull();
  });

  it("omits the location when the backend has no backup directory", () => {
    databaseState.value = { backup_directory: "" };

    render(createElement(DataStorageSettings));

    expect(screen.queryByText(/VACUUM INTO snapshots stored under/)).toBeNull();
  });
});

/** "Feature Toggles" -> "featureToggles", matching the catalog key suffix. */
function camelRoute(title: string): string {
  const [first, ...rest] = title.split(" ");
  return first.toLowerCase() + rest.join("");
}
