import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { SyncDiff } from "@/lib/api/domains/office-api";
import { SyncDiffPane } from "./sync-diff-pane";

function diffWithChanges(): SyncDiff {
  return {
    direction: "incoming",
    preview: {
      agents: { created: ["ceo"], updated: [], deleted: [] },
      skills: { created: [], updated: [], deleted: [] },
      routines: { created: [], updated: [], deleted: [] },
      projects: { created: [], updated: [], deleted: [] },
    },
  };
}

describe("SyncDiffPane — apply-incoming/apply-outgoing guard (AC-OFFICE-CONFIG-SYNC-006.6)", () => {
  afterEach(cleanup);

  it("enables Apply when there are pending changes and no disabled reason", () => {
    render(
      <SyncDiffPane
        title="Incoming"
        description="desc"
        icon={null}
        diff={diffWithChanges()}
        loading={false}
        applying={false}
        applyLabel="Import"
        onApply={vi.fn()}
      />,
    );
    expect((screen.getByRole("button", { name: "Import" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("disables Apply and shows the reason when config sync is the active source", () => {
    render(
      <SyncDiffPane
        title="Incoming"
        description="desc"
        icon={null}
        diff={diffWithChanges()}
        loading={false}
        applying={false}
        applyLabel="Import"
        onApply={vi.fn()}
        disabledReason="Config sync is the active configuration source for this workspace."
      />,
    );
    expect((screen.getByRole("button", { name: "Import" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(
      screen.getByText("Config sync is the active configuration source for this workspace."),
    ).toBeTruthy();
  });

  it("still renders the read-only diff while Apply is disabled", () => {
    render(
      <SyncDiffPane
        title="Incoming"
        description="desc"
        icon={null}
        diff={diffWithChanges()}
        loading={false}
        applying={false}
        applyLabel="Import"
        onApply={vi.fn()}
        disabledReason="Config sync is the active configuration source for this workspace."
      />,
    );
    expect(screen.getByText("+ ceo")).toBeTruthy();
  });
});
