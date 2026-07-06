import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

import { AgentProfileDeleteConflictDialog } from "./agent-profile-delete-dialog";

afterEach(cleanup);

describe("AgentProfileDeleteConflictDialog", () => {
  it("renders the watcher list grouped by kind on a watcher-only conflict", () => {
    render(
      <AgentProfileDeleteConflictDialog
        conflict={{
          activeSessions: [],
          watchers: [
            { id: "linear-w1", kind: "linear", label: "team ENG" },
            { id: "linear-w2", kind: "linear", label: "team WEB" },
            { id: "github-w1", kind: "github_issue", label: "kdlbs/kandev" },
          ],
          routingTiers: [],
        }}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />,
    );

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
    render(
      <AgentProfileDeleteConflictDialog
        conflict={{
          activeSessions: [{ task_id: "t1", task_title: "Live task", is_ephemeral: false }],
          watchers: [],
          routingTiers: [],
        }}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />,
    );

    expect(screen.getByText(/Tasks:/)).toBeTruthy();
    expect(screen.getByText("Live task")).toBeTruthy();
    expect(screen.queryByText(/Watchers \(will be disabled\)/)).toBeNull();
  });

  it("renders both sections when sessions and watchers coexist", () => {
    render(
      <AgentProfileDeleteConflictDialog
        conflict={{
          activeSessions: [{ task_id: "t1", task_title: "Live task", is_ephemeral: false }],
          watchers: [{ id: "jira-w1", kind: "jira", label: "project = ENG" }],
          routingTiers: [],
        }}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />,
    );

    expect(screen.getByText("Live task")).toBeTruthy();
    expect(screen.getByText(/Jira:/)).toBeTruthy();
    expect(screen.getByText(/project = ENG/)).toBeTruthy();
  });

  it("renders tier mappings as a hard blocker", () => {
    render(
      <AgentProfileDeleteConflictDialog
        conflict={{
          activeSessions: [],
          watchers: [],
          routingTiers: [{ workspace_id: "ws-1", provider_id: "codex-acp", tier: "balanced" }],
        }}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />,
    );

    expect(screen.getByText(/Cannot delete agent profile/i)).toBeTruthy();
    expect(screen.getByText(/Workspace tier mappings:/)).toBeTruthy();
    expect(screen.getByText(/ws-1: codex-acp balanced/)).toBeTruthy();
    expect(screen.queryByText(/Delete Anyway/)).toBeNull();
  });

  it("does not render the dialog when conflict is null", () => {
    render(
      <AgentProfileDeleteConflictDialog
        conflict={null}
        onOpenChange={() => {}}
        onConfirm={() => {}}
      />,
    );

    expect(screen.queryByText(/Delete agent profile/i)).toBeNull();
  });
});
