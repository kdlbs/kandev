import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";
import { WorkflowSyncStatusCard } from "./workflow-sync-status-banner";

/**
 * The headline is a `<Trans>` whose `<1>` addresses the bold `<span>`
 * positionally, so a prettier reflow can silently reassemble it into empty
 * fragments without failing anything (see docs/i18n.md). The assertions below
 * reconstruct the whole sentence and check the repo slug is inside the bold
 * element, which is what a drifted index would break.
 *
 * The metadata line is the other shape worth pinning: it is joined from three
 * separately-translated parts, and the repo path / interval are values.
 */

function config(overrides: Partial<WorkflowSyncConfig> = {}): WorkflowSyncConfig {
  // No `as` cast: the fixture must satisfy the real contract, so a field added
  // to WorkflowSyncConfig fails here rather than being silently absent.
  // `last_synced_at` is left off — it is optional and absent until the first
  // sync attempt, which is the state most of these cases exercise.
  return {
    workspace_id: "workspace-1",
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    project_path: "",
    branch: "main",
    path: ".kandev/workflows",
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

describe("WorkflowSyncStatusCard", () => {
  afterEach(cleanup);

  it("keeps the repository slug inside the bold span of the headline", () => {
    render(<WorkflowSyncStatusCard config={config()} syncing={false} onSyncNow={vi.fn()} />);
    const slug = screen.getByText("kdlbs/kandev");
    expect(slug.className).toContain("font-semibold");
    expect(slug.parentElement?.textContent).toBe("Syncing from kdlbs/kandev");
  });

  it("joins the metadata parts while auto-sync is on and no sync has run", () => {
    render(<WorkflowSyncStatusCard config={config()} syncing={false} onSyncNow={vi.fn()} />);
    expect(
      screen.getByText("Directory .kandev/workflows · every 300s · waiting for first sync…"),
    ).toBeTruthy();
  });

  // The interval is a count, so it goes through i18next plural selection rather
  // than being interpolated as an opaque value.
  it("selects the singular interval form for one second", () => {
    render(
      <WorkflowSyncStatusCard
        config={config({ interval_seconds: 1 })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(
      screen.getByText("Directory .kandev/workflows · every 1s · waiting for first sync…"),
    ).toBeTruthy();
  });

  // `formatRelative` routes its buckets through i18next; date-fns would render
  // English inside a translated sentence.
  it("renders the last-sync time through the locale-aware relative formatter", () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
    render(
      <WorkflowSyncStatusCard
        config={config({ last_synced_at: threeHoursAgo })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText(/last synced 3h ago$/)).toBeTruthy();
  });

  it("labels a failed attempt separately from a successful sync", () => {
    render(
      <WorkflowSyncStatusCard
        config={config({ last_ok: false, last_synced_at: new Date().toISOString() })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText(/last attempt just now$/)).toBeTruthy();
  });

  it("labels an empty directory as the repository root and reports auto-sync off", () => {
    render(
      <WorkflowSyncStatusCard
        config={config({ path: "", poll_enabled: false })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(
      screen.getByText(
        "Directory (repository root) · auto-sync off · not synced yet — use Sync now",
      ),
    ).toBeTruthy();
  });

  it("falls back to a generic message when a failed sync carries no error text", () => {
    render(
      <WorkflowSyncStatusCard
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
      <WorkflowSyncStatusCard
        config={config({ last_warnings: ["skipped bad-workflow.yml: invalid step id"] })}
        syncing={false}
        onSyncNow={vi.fn()}
      />,
    );
    expect(screen.getByText("skipped bad-workflow.yml: invalid step id")).toBeTruthy();
  });
});
