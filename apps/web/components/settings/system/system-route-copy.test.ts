import { cleanup, render, screen } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { t } from "@/lib/i18n";
import { DataLogsSettings } from "./data-logs-settings";
import { BACKUP_SQL_COMMAND } from "./system-route-shell";

const databaseState = vi.hoisted(() => ({ value: null as unknown }));
const databaseStatsState = vi.hoisted(() => ({
  result: {
    database: null as { backup_directory?: string } | null,
    isLoading: false,
    error: null as string | null,
    reload: () => Promise.resolve(),
    retry: () => Promise.resolve(),
  } as unknown,
  calls: 0,
  cardResult: undefined as unknown,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { system: { database: unknown } }) => unknown) =>
    selector({ system: { database: databaseState.value } }),
}));
vi.mock("@/hooks/domains/system/use-database-stats", () => ({
  useDatabaseStats: () => {
    databaseStatsState.calls += 1;
    return databaseStatsState.result;
  },
}));
vi.mock("@/components/settings/settings-target", () => ({
  SettingsTarget: ({ children }: { children?: ReactNode }) => children ?? null,
}));
vi.mock("./backups-table", () => ({ BackupsTable: () => null }));
vi.mock("./database-stats-card", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  return {
    DatabaseStatsCard: ({
      stats,
    }: {
      stats?: { database?: { backup_directory?: string } | null };
    }) => {
      databaseStatsState.cardResult = stats;
      return React.createElement(
        "output",
        { "data-testid": "database-card-backup-directory" },
        stats?.database?.backup_directory ?? "",
      );
    },
  };
});
vi.mock("./log-viewer", () => ({ LogViewer: () => null }));
vi.mock("./retention-settings-card", () => ({ RetentionSettingsCard: () => null }));
vi.mock("./tool-payload-retention-card", () => ({ ToolPayloadRetentionCard: () => null }));
afterEach(() => {
  cleanup();
  databaseState.value = null;
  databaseStatsState.result = {
    database: null,
    isLoading: false,
    error: null,
    reload: () => Promise.resolve(),
    retry: () => Promise.resolve(),
  };
  databaseStatsState.calls = 0;
  databaseStatsState.cardResult = undefined;
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
    // /settings/system/logs now redirects into Data & Logs, whose Logs
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

describe("Data & Logs backup location copy", () => {
  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
  it("renders the resolved SQLite backup directory", () => {
    const path = "/var/lib/kandev/backups";
    const queryResult = {
      database: { backup_directory: path },
      isLoading: false,
      error: null,
      reload: vi.fn(),
      retry: vi.fn(),
    };
    databaseStatsState.result = queryResult;

    render(createElement(DataLogsSettings));

    expect(screen.getByText(`VACUUM INTO snapshots stored under ${path}.`)).toBeTruthy();
    expect(screen.getByTestId("database-card-backup-directory").textContent).toBe(path);
    expect(databaseStatsState.calls).toBe(1);
    expect(databaseStatsState.cardResult).toBe(queryResult);
  });

  it("omits the location when database information is unavailable", () => {
    databaseStatsState.result = {
      database: null,
      isLoading: false,
      error: null,
      reload: vi.fn(),
      retry: vi.fn(),
    };
    render(createElement(DataLogsSettings));

    expect(screen.queryByText(/VACUUM INTO snapshots stored under/)).toBeNull();
  });

  it("omits the location when the backend has no backup directory", () => {
    databaseStatsState.result = {
      database: { backup_directory: "" },
      isLoading: false,
      error: null,
      reload: vi.fn(),
      retry: vi.fn(),
    };

    render(createElement(DataLogsSettings));

    expect(screen.queryByText(/VACUUM INTO snapshots stored under/)).toBeNull();
  });
});

describe("Data & Logs composition", () => {
  it("keeps database, backups, and logs without the Storage section", () => {
    render(createElement(DataLogsSettings));

    expect(screen.getAllByText(t("system:navDatabase")).length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByText(t("system:navRetention"))).toBeNull();
    expect(screen.getByRole("heading", { name: t("system:navBackups") })).toBeTruthy();
    expect(screen.getByRole("tab", { name: t("system:navLogs") })).toBeTruthy();
    expect(screen.queryByText(t("system:storageTitle"))).toBeNull();
  });

  it("describes only database statistics, backups, and server logs", () => {
    expect(t("system:dataStoragePageDescription")).toBe(
      "Database statistics, backups, and server logs.",
    );
  });
});

/** "Feature Toggles" -> "featureToggles", matching the catalog key suffix. */
function camelRoute(title: string): string {
  const [first, ...rest] = title.split(" ");
  return first.toLowerCase() + rest.join("");
}
