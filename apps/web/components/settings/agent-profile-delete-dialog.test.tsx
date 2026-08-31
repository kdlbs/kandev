import { describe, it, expect, afterEach, vi } from "vitest";
import { fireEvent, render, screen, cleanup, waitFor, within } from "@testing-library/react";
import { useRef, useState } from "react";

import { StateProvider } from "@/components/state-provider";
import {
  AgentProfileDeleteConfirmation,
  AgentProfileDeleteConflictDialog,
} from "./agent-profile-delete-dialog";
import type { AgentProfileDeleteConflict } from "./agent-profile-delete-dialog";

afterEach(cleanup);

function renderLocalConfirmation(
  isFinePointer: boolean,
  onConfirm: () => void | Promise<void> = () => {},
) {
  function Harness() {
    const [open, setOpen] = useState(true);
    const anchorRef = useRef<HTMLButtonElement>(null);
    const close = () => setOpen(false);
    return (
      <>
        <button ref={anchorRef} type="button" data-testid="profile-delete-trigger">
          Delete
        </button>
        <AgentProfileDeleteConfirmation
          open={open}
          isFinePointer={isFinePointer}
          anchorRef={anchorRef}
          onOpenChange={setOpen}
          onCancel={close}
          onConfirm={onConfirm}
        />
      </>
    );
  }

  return render(<Harness />);
}

describe("AgentProfileDeleteConfirmation", () => {
  it("uses touch-sized inline actions on coarse pointers and cancels without mutating", async () => {
    const onConfirm = vi.fn();
    renderLocalConfirmation(false, onConfirm);

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })),
    );
    expect(screen.getByTestId("agent-profile-delete-inline-confirmation")).toBeTruthy();
    const inlineConfirmation = screen.getByTestId("agent-profile-delete-inline-confirmation");
    expect(within(inlineConfirmation).getByRole("button", { name: "Delete" }).className).toContain(
      "h-11",
    );

    fireEvent.click(within(inlineConfirmation).getByRole("button", { name: "Cancel" }));

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.queryByTestId("agent-profile-delete-inline-confirmation")).toBeNull();
  });

  it("uses a non-modal fine-pointer popover and confirms exactly once after closing", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByTestId("agent-profile-delete-confirm-popover") === null;
    });
    renderLocalConfirmation(true, onConfirm);

    const popover = screen.getByTestId("agent-profile-delete-confirm-popover");
    expect(popover).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(within(popover).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(shellClosed).toBe(true);
  });
});

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
  // An automation is configuration, not a session: nothing is running, so it
  // never reaches activeSessions. Before this was rendered, an automation-only
  // conflict popped a dialog that listed nothing at all and then offered
  // "Delete Anyway".
  it("names the automations on an automation-only conflict", () => {
    renderConflictDialog({
      activeSessions: [],
      watchers: [],
      routingTiers: [],
      automations: [{ id: "auto-1", name: "Nightly dependency sweep", workspace_id: "ws-1" }],
    });

    expect(screen.getByTestId("delete-conflict-automations")).toBeTruthy();
    screen.getByText("Nightly dependency sweep");
  });

  it("renders the watcher list grouped by kind on a watcher-only conflict", () => {
    renderConflictDialog({
      activeSessions: [],
      watchers: [
        { id: "linear-w1", kind: "linear", label: "team ENG" },
        { id: "linear-w2", kind: "linear", label: "team WEB" },
        { id: "github-w1", kind: "github_issue", label: "kdlbs/kandev" },
      ],
      routingTiers: [],
      automations: [],
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
      automations: [],
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
      automations: [],
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
      automations: [],
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

  // The watcher `kind` is the wire enum and its label is a catalog key, so a
  // renamed key or a dropped entry must fail here rather than silently render
  // the raw enum value to the user.
  it("resolves every known watcher kind to its label, and echoes an unknown one", () => {
    renderConflictDialog({
      activeSessions: [],
      watchers: [
        { id: "w1", kind: "linear", label: "team ENG" },
        { id: "w2", kind: "jira", label: "project = ENG" },
        { id: "w3", kind: "github_issue", label: "kdlbs/kandev" },
        { id: "w4", kind: "github_review", label: "kdlbs/kandev PRs" },
        // A kind the frontend does not know yet — the raw value is echoed
        // rather than rendering an empty label.
        { id: "w5", kind: "sentry" as never, label: "proj/backend" },
      ],
      routingTiers: [],
      automations: [],
    });

    expect(screen.getByText(/Linear:/)).toBeTruthy();
    expect(screen.getByText(/Jira:/)).toBeTruthy();
    expect(screen.getByText(/GitHub Issues:/)).toBeTruthy();
    expect(screen.getByText(/GitHub PR Reviews:/)).toBeTruthy();
    expect(screen.getByText(/sentry:/)).toBeTruthy();
  });

  it("does not render the dialog when conflict is null", () => {
    renderConflictDialog(null);

    expect(screen.queryByText(/Delete agent profile/i)).toBeNull();
  });
});

describe("AgentProfileDeleteConflictDialog layout", () => {
  it("keeps conflict details scrollable and decisions outside the scroll region", () => {
    renderConflictDialog({
      activeSessions: [{ task_id: "t1", task_title: "Live task", is_ephemeral: false }],
      watchers: [],
      routingTiers: [],
      automations: [],
    });

    const dialog = screen.getByTestId("agent-profile-delete-conflict-dialog");
    const body = screen.getByTestId("agent-profile-delete-conflict-body");
    const footer = screen.getByTestId("agent-profile-delete-conflict-footer");

    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(dialog.contains(body)).toBe(true);
    expect(body.contains(footer)).toBe(false);
    expect(within(footer).getByRole("button", { name: "Cancel" })).toBeTruthy();
  });

  // @covers AC-UI-SURFACE-TEXT-HIERARCHY-001.2, AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  it("keeps long conflict labels in one left-aligned semantic description", () => {
    const longTaskTitle = `Profile task ${"x".repeat(320)}`;
    renderConflictDialog({
      activeSessions: [{ task_id: "long-task", task_title: longTaskTitle, is_ephemeral: false }],
      watchers: [],
      routingTiers: [],
      automations: [],
    });

    const body = screen.getByTestId("agent-profile-delete-conflict-body");
    expect(body.className).toContain("min-w-0");
    expect(body.className).toContain("text-left");
    expect(body.className).toContain("space-y-2");
    const directParagraphs = body.querySelectorAll(":scope > p");
    expect(directParagraphs).toHaveLength(2);
    expect(directParagraphs[1]?.className).not.toContain("mt-2");

    const taskItem = screen.getByText(longTaskTitle);
    expect(taskItem.tagName).toBe("LI");
  });
});
