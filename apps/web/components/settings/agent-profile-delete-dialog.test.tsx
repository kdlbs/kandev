import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

import { StateProvider } from "@/components/state-provider";
import { AgentProfileDeleteConflictDialog } from "./agent-profile-delete-dialog";
import type { AgentProfileDeleteConflict } from "./agent-profile-delete-dialog";

afterEach(cleanup);

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00.000Z";

function renderConflictDialog(conflict: AgentProfileDeleteConflict | null) {
  return render(
    <StateProvider
      initialState={{
        workspaces: {
          activeId: "ws-1",
          items: [
            {
              id: "ws-1",
              name: "Office Workspace",
              owner_id: "user-1",
              created_at: FIXTURE_TIMESTAMP,
              updated_at: FIXTURE_TIMESTAMP,
            },
          ],
        },
        settingsAgents: {
          items: [
            {
              id: "codex-acp",
              name: "Codex",
              supports_mcp: false,
              profiles: [],
              created_at: FIXTURE_TIMESTAMP,
              updated_at: FIXTURE_TIMESTAMP,
            },
          ],
        },
      }}
    >
      <AgentProfileDeleteConflictDialog
        conflict={conflict}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />
    </StateProvider>,
  );
}

describe("AgentProfileDeleteConflictDialog", () => {
  it("renders the watcher list grouped by kind on a watcher-only conflict", () => {
    renderConflictDialog({
      activeSessions: [],
      watchers: [
        { id: "linear-w1", kind: "linear", label: "team ENG" },
        { id: "linear-w2", kind: "linear", label: "team WEB" },
        { id: "github-w1", kind: "github_issue", label: "kdlbs/kandev" },
      ],
      routingTiers: [],
    });

    // Watcher group headings render the human-friendly kind label, and
    // each watcher's label string appears once. Critically: the dialog
    // pops even with no active sessions — this is the bug class the
    // backend self-heal pre-flight fix would have left unaddressed
    // without the frontend wiring.
    expect(screen.getByText(/Watchers \(will be disabled\)/)).toBeTruthy();
    expect(screen.getByText(/Linear:/)).toBeTruthy();
    expect(screen.getByText(/GitHub Issues:/)).toBeTruthy();
    expect(screen.getByText(/team ENG/)).toBeTruthy();
    expect(screen.getByText(/team WEB/)).toBeTruthy();
    expect(screen.getByText(/kdlbs\/kandev/)).toBeTruthy();
  });

  it("does not render the watchers section when the conflict is sessions-only", () => {
    renderConflictDialog({
      activeSessions: [{ task_id: "t1", task_title: "Live task", is_ephemeral: false }],
      watchers: [],
      routingTiers: [],
    });

    expect(screen.getByText(/Tasks:/)).toBeTruthy();
    expect(screen.getByText("Live task")).toBeTruthy();
    expect(screen.queryByText(/Watchers \(will be disabled\)/)).toBeNull();
  });

  it("renders both sections when sessions and watchers coexist", () => {
    renderConflictDialog({
      activeSessions: [{ task_id: "t1", task_title: "Live task", is_ephemeral: false }],
      watchers: [{ id: "jira-w1", kind: "jira", label: "project = ENG" }],
      routingTiers: [],
    });

    expect(screen.getByText("Live task")).toBeTruthy();
    expect(screen.getByText(/Jira:/)).toBeTruthy();
    expect(screen.getByText(/project = ENG/)).toBeTruthy();
  });

  it("renders tier mappings as a hard blocker", () => {
    renderConflictDialog({
      activeSessions: [],
      watchers: [],
      routingTiers: [{ workspace_id: "ws-1", provider_id: "codex-acp", tier: "balanced" }],
    });

    expect(screen.getByText(/Cannot delete agent profile/i)).toBeTruthy();
    expect(screen.getByText(/Workspace tier mappings:/)).toBeTruthy();
    expect(
      screen.getByText((_content, element) =>
        Boolean(
          element?.tagName === "LI" &&
          element.textContent?.includes(
            "Balanced tier in Office Workspace (ws-1) for Codex (codex-acp)",
          ),
        ),
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/Delete Anyway/)).toBeNull();
  });

  it("does not render the dialog when conflict is null", () => {
    renderConflictDialog(null);

    expect(screen.queryByText(/Delete agent profile/i)).toBeNull();
  });
});
