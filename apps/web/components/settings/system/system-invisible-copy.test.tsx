import { cleanup, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { activateLocale, t } from "@/lib/i18n";
import type { HealthCheckSummary, HealthIssue } from "@/lib/types/health";
import { HealthIssuesCard } from "./health-issues-card";
import { JobProgressIndicator } from "./job-progress-indicator";
import { SystemGroup } from "@/components/app-sidebar/sections/settings/system-group";

afterEach(cleanup);

const jobs = vi.hoisted(() => ({ current: [] as unknown[] }));
vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: () => null,
  useSystemJobs: () => jobs.current,
}));

const health = vi.hoisted(() => ({
  issues: [] as HealthIssue[],
  checks: [] as HealthCheckSummary[],
  loaded: true,
}));
vi.mock("@/hooks/domains/settings/use-system-health", () => ({
  useSystemHealth: () => health,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) =>
    selector({ workspaces: { activeId: "ws-1" }, auth: { user: { role: "admin" } } }),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));

/**
 * `stateLabel()` returns its four labels from a plain function, so
 * `mode: "jsx-only"` reported this file as clean while every label was a
 * hardcoded literal. It renders on four cards — including
 * `storage-quarantine-card.tsx`, which ships on a route this PR does not
 * otherwise touch — so a regression here is not visible from the System pages.
 */
describe("JobProgressIndicator state labels", () => {
  const job = (state: string) => ({
    id: "j1",
    kind: "vacuum",
    state,
    started_at: "2026-08-03T10:00:00Z",
    message: "",
  });

  it.each([
    ["queued", "Queued"],
    ["running", "Running"],
    ["succeeded", "Done"],
    ["failed", "Failed"],
  ])("renders %s as %s", (state, label) => {
    jobs.current = [job(state)];
    render(<JobProgressIndicator kind="vacuum" />);
    expect(screen.getByTestId("system-job-vacuum").textContent).toContain(label);
  });

  // The wire enum is echoed rather than rendering blank, so a state from a
  // newer backend still shows something.
  it("echoes an unknown state token", () => {
    jobs.current = [job("throttled")];
    render(<JobProgressIndicator kind="vacuum" />);
    expect(screen.getByTestId("system-job-vacuum").textContent).toContain("throttled");
  });
});

/**
 * This badge used to build its plural by hand — `issue${n === 1 ? "" : "s"}` —
 * which renders correctly in English and is untranslatable everywhere else.
 */
describe("HealthIssuesCard issue count", () => {
  it("agrees the noun with the number", () => {
    health.issues = [
      { id: "a", severity: "error", title: "Disk full", message: "", fix_url: "", fix_label: "" },
    ] as HealthIssue[];
    render(<HealthIssuesCard />);
    expect(screen.getByTestId("system-health-card").textContent).toContain("1 issue");

    cleanup();
    health.issues = [
      { id: "a", severity: "error", title: "Disk full", message: "", fix_url: "", fix_label: "" },
      { id: "b", severity: "warning", title: "Slow", message: "", fix_url: "", fix_label: "" },
    ] as HealthIssue[];
    render(<HealthIssuesCard />);
    expect(screen.getByTestId("system-health-card").textContent).toContain("2 issues");
  });

  // `severity` is a wire enum. Title-casing the raw token was English-shaped by
  // accident; unknown severities still echo rather than render blank.
  it("labels a known severity and echoes an unknown one", () => {
    health.issues = [
      { id: "a", severity: "warning", title: "Slow", message: "", fix_url: "", fix_label: "" },
      {
        id: "b",
        severity: "critical" as HealthIssue["severity"],
        title: "Boom",
        message: "",
        fix_url: "",
        fix_label: "",
      },
    ] as HealthIssue[];
    render(<HealthIssuesCard />);
    const card = screen.getByTestId("system-health-card").textContent ?? "";
    expect(card).toContain("Warning");
    expect(card).toContain("critical");
  });
});

/**
 * `BASE_ITEMS` / `AUTH_ITEMS` are SCREAMING_CASE, which the guard skips
 * entirely: it reported one finding here (`label="System"`) while ten nav
 * labels sat in those two tables. They now resolve at render from `labelKey`.
 */
describe("SystemGroup nav labels", () => {
  it("renders every System route label through the catalog", () => {
    render(<SystemGroup pathname="/settings/system/status" expanded />);
    const labels = [
      "Status",
      "Feature Toggles",
      "Database",
      "Backups",
      "Storage",
      "Logs",
      "Updates",
      "About",
      "Licenses",
      "Users",
    ];
    for (const label of labels) {
      expect(screen.getByText(label)).toBeTruthy();
    }
  });
});

/**
 * The English assertions above pin the copy, but they cannot tell a `t()` call
 * from a literal that happens to read the same — reverting
 * `t("system:jobStateQueued")` to `"Queued"` passes every one of them. Only a
 * non-English locale separates the two, and both surfaces here were reported as
 * clean by lint, so this is the check that keeps them migrated.
 */
describe("copy the guard cannot see, under the pseudo-locale", () => {
  const ACCENTED = /[À-ɏ]/;

  beforeAll(async () => {
    await activateLocale("pseudo");
  });
  afterAll(async () => {
    await activateLocale("en");
  });

  it("accents every job state label", () => {
    for (const state of ["queued", "running", "succeeded", "failed"]) {
      jobs.current = [
        { id: "j1", kind: "vacuum", state, started_at: "2026-08-03T10:00:00Z", message: "" },
      ];
      render(<JobProgressIndicator kind="vacuum" />);
      expect(screen.getByTestId("system-job-vacuum").textContent).toMatch(ACCENTED);
      cleanup();
    }
  });

  /**
   * The Backups description carries a SQL statement and a filesystem path. Baked
   * into the message they render `VÀĆŨŨḾ ĨŃŢŌ` and `<ďàţà-ďĩŕ>/ƀàćķũƥś/` — a
   * command the user runs and a directory they have to find, both turned into
   * dead pointers. They are interpolated as values instead, so the frame
   * accents and they do not.
   */
  it("keeps the backup SQL command and path literal inside a translated frame", () => {
    const description = t("system:backupsPageDescription", {
      command: "VACUUM INTO",
      path: "<data-dir>/backups/",
    });
    expect(description).toContain("VACUUM INTO");
    expect(description).toContain("<data-dir>/backups/");
    // The surrounding sentence is still translated.
    expect(description).toMatch(ACCENTED);
  });

  it("accents every System nav label", () => {
    render(<SystemGroup pathname="/settings/system/status" expanded />);
    const leaves = document.querySelectorAll("a");
    expect(leaves.length).toBeGreaterThan(0);
    for (const leaf of leaves) {
      expect(leaf.textContent ?? "").toMatch(ACCENTED);
    }
  });
});
