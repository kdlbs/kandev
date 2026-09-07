import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { OfficeConfigSyncConfig } from "@/lib/types/office-config-sync";
import { OfficeConfigSyncStatusCard } from "./office-config-sync-status-card";

/**
 * The headline is a `<Trans>` whose `<1>` addresses the bold `<span>`
 * positionally (see docs/i18n.md) — the assertions below reconstruct the
 * whole sentence and check the repository slug sits inside the bold element,
 * which is exactly what a drifted index would break.
 */

function config(overrides: Partial<OfficeConfigSyncConfig> = {}): OfficeConfigSyncConfig {
  // No `as` cast: the fixture must satisfy the real contract, so a field
  // added to OfficeConfigSyncConfig fails here rather than being silently
  // absent. `last_synced_at` is left off — it is optional and absent until
  // the first sync attempt, which is the state most cases below exercise.
  return {
    workspace_id: "workspace-1",
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: "kandev-office-config",
    project_path: "",
    branch: "main",
    path: "",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    last_error: "",
    last_warnings: [],
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    ...overrides,
  };
}

describe("OfficeConfigSyncStatusCard", () => {
  afterEach(cleanup);

  it("keeps the repository slug inside the bold span of the headline", () => {
    render(<OfficeConfigSyncStatusCard config={config()} syncing={false} onSyncNow={vi.fn()} />);
    const slug = screen.getByText("kdlbs/kandev-office-config");
    expect(slug.className).toContain("font-semibold");
    expect(slug.parentElement?.textContent).toBe("Syncing from kdlbs/kandev-office-config");
  });

  it("labels a GitLab source by its project path, not owner/name", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({
          provider: "gitlab",
          repo_owner: "",
          repo_name: "",
          project_path: "group/project",
        })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("group/project")).toBeTruthy();
  });

  it("joins the metadata parts while auto-sync is on and no sync has run", () => {
    render(<OfficeConfigSyncStatusCard config={config()} syncing={false} onSyncNow={vi.fn()} />);
    expect(
      screen.getByText("Directory: Repository root · every 300 seconds · Waiting for first sync"),
    ).toBeTruthy();
  });

  it("selects the singular interval form for one second", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({ interval_seconds: 1 })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(
      screen.getByText("Directory: Repository root · every 1 second · Waiting for first sync"),
    ).toBeTruthy();
  });
});

describe("OfficeConfigSyncStatusCard — sync status and warnings", () => {
  afterEach(cleanup);

  it("renders the last-sync time through the locale-aware relative formatter", () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
    render(
      <OfficeConfigSyncStatusCard
        config={config({ last_synced_at: threeHoursAgo })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText(/Last synced 3h ago$/)).toBeTruthy();
  });

  it("labels a failed attempt separately from a successful sync", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({ last_ok: false, last_synced_at: new Date().toISOString() })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText(/Last attempt just now$/)).toBeTruthy();
  });

  it("labels an empty directory as the repository root and reports auto-sync off", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({ path: "", poll_enabled: false })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(
      screen.getByText("Directory: Repository root · automatic sync off · Not synced yet"),
    ).toBeTruthy();
  });

  it("falls back to a generic message when a failed sync carries no error text", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({
          last_ok: false,
          last_synced_at: new Date().toISOString(),
          last_error: "",
        })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("Sync failed")).toBeTruthy();
  });

  it("renders backend warnings verbatim — they are server text, not catalog copy", () => {
    render(
      <OfficeConfigSyncStatusCard
        config={config({ last_warnings: ["skipped bad-agent.yaml: invalid identity"] })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("skipped bad-agent.yaml: invalid identity")).toBeTruthy();
  });

  it("caps rendered warnings at 10 and reports the remainder count", () => {
    const warnings = Array.from({ length: 13 }, (_, i) => `warning ${i + 1}`);
    render(
      <OfficeConfigSyncStatusCard
        config={config({ last_warnings: warnings })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("warning 1")).toBeTruthy();
    expect(screen.getByText("warning 10")).toBeTruthy();
    expect(screen.queryByText("warning 11")).toBeNull();
    expect(screen.getByText("3 more warnings")).toBeTruthy();
  });

  it("does not render a remainder note when there are 10 or fewer warnings", () => {
    const warnings = Array.from({ length: 10 }, (_, i) => `warning ${i + 1}`);
    render(
      <OfficeConfigSyncStatusCard
        config={config({ last_warnings: warnings })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("warning 10")).toBeTruthy();
    expect(screen.queryByText(/more warning/)).toBeNull();
  });

  it("disables Sync now while a sync is already in flight", () => {
    render(<OfficeConfigSyncStatusCard config={config()} syncing={true} onSyncNow={vi.fn()} />);
    expect((screen.getByTestId("office-config-sync-now") as HTMLButtonElement).disabled).toBe(true);
  });
});
