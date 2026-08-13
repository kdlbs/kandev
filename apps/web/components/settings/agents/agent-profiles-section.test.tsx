import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent, AgentProfile } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  deleteAgentProfileAction: vi.fn(),
  duplicateAgentProfileAction: vi.fn(),
  toast: vi.fn(),
  routerPush: vi.fn(),
}));

function profile(id: string, name: string): AgentProfile {
  return { id, name, agentDisplayName: "Claude Code" } as AgentProfile;
}

const AGENT = {
  id: "agent-1",
  name: "claude",
  profiles: [profile("p-1", "Alpha"), profile("p-2", "Beta")],
} as unknown as Agent;

// A minimal store: the component must read it at write time, so the test needs
// real read-after-write semantics rather than a frozen snapshot.
let storeState: {
  settingsAgents: { items: Agent[] };
  agentProfiles: { items: Array<{ id: string }> };
};

function setSettingsAgents(items: Agent[]) {
  storeState.settingsAgents.items = items;
}
function setAgentProfiles(items: Array<{ id: string }>) {
  storeState.agentProfiles.items = items;
}

const selectors = {
  setSettingsAgents,
  setAgentProfiles,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: unknown) => unknown) => selector({ ...storeState, ...selectors }),
  useAppStoreApi: () => ({ getState: () => storeState, setState: vi.fn() }),
}));

vi.mock("@/app/actions/agents", () => ({
  deleteAgentProfileAction: mocks.deleteAgentProfileAction,
  duplicateAgentProfileAction: mocks.duplicateAgentProfileAction,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
  usePathname: () => "/settings/agents",
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode }) => <a {...props}>{children}</a>,
}));

// Radix's dropdown does not open under jsdom clicks, and the confirm dialog
// portals out of the row. Both are chrome around the behaviour under test, so
// render them inline and reachable.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children?: ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({
    children,
    onSelect,
    "data-testid": testId,
  }: {
    children?: ReactNode;
    onSelect?: () => void;
    "data-testid"?: string;
  }) => (
    <button type="button" data-testid={testId ?? "delete-item"} onClick={onSelect}>
      {children}
    </button>
  ),
}));

vi.mock("@/components/settings/agent-profile-delete-dialog", () => ({
  AgentProfileDeleteConfirmDialog: ({
    open,
    onConfirm,
  }: {
    open: boolean;
    onConfirm: () => void;
  }) =>
    open ? (
      <button type="button" data-testid="confirm-delete" onClick={onConfirm}>
        confirm
      </button>
    ) : null,
}));

import { AgentProfilesSubList, ProfileRow } from "./agent-profiles-section";

function renderRows() {
  return render(
    <>
      {AGENT.profiles.map((p) => (
        <ProfileRow key={p.id} agent={AGENT} profile={p} />
      ))}
    </>,
  );
}

function confirmDeleteFor(name: string) {
  const row = screen.getByLabelText(name).closest('[data-testid="agent-profile-row"]');
  if (!row) throw new Error(`no row for ${name}`);
  fireEvent.click(row.querySelector('[data-testid="delete-item"]')!);
  fireEvent.click(row.querySelector('[data-testid="confirm-delete"]')!);
}

describe("ProfileRow deletion", () => {
  beforeEach(() => {
    storeState = {
      settingsAgents: { items: [{ ...AGENT, profiles: [...AGENT.profiles] }] },
      agentProfiles: { items: [{ id: "p-1" }, { id: "p-2" }] },
    };
    vi.clearAllMocks();
  });
  afterEach(() => cleanup());

  it("keeps the flattened profile options in step with the agent list", async () => {
    mocks.deleteAgentProfileAction.mockResolvedValue({ status: "ok" });
    renderRows();

    confirmDeleteFor("Alpha");

    await waitFor(() => {
      expect(storeState.settingsAgents.items[0].profiles.map((p) => p.id)).toEqual(["p-2"]);
    });
    // Skipping this slice left the deleted profile selectable in every picker
    // until a reload, because its only refetch is a one-shot.
    expect(storeState.agentProfiles.items.map((p) => p.id)).toEqual(["p-2"]);
  });

  it("does not resurrect a profile when two deletions overlap", async () => {
    // Both rows capture their state before either write lands: the first
    // deletion resolves only after the second has been requested.
    const gate: Array<() => void> = [];
    mocks.deleteAgentProfileAction.mockImplementation(
      () => new Promise((resolve) => gate.push(() => resolve({ status: "ok" }))),
    );

    renderRows();
    confirmDeleteFor("Alpha");
    confirmDeleteFor("Beta");
    await waitFor(() => expect(gate).toHaveLength(2));

    gate[0]();
    await waitFor(() => {
      expect(storeState.settingsAgents.items[0].profiles.map((p) => p.id)).toEqual(["p-2"]);
    });
    gate[1]();

    await waitFor(() => {
      expect(storeState.settingsAgents.items[0].profiles).toEqual([]);
    });
    expect(storeState.agentProfiles.items).toEqual([]);
  });
});

describe("ProfileRow duplicate", () => {
  beforeEach(() => {
    storeState = {
      settingsAgents: { items: [{ ...AGENT, profiles: [...AGENT.profiles] }] },
      agentProfiles: { items: [] },
    };
  });

  it("calls the duplicate action with the profile id", async () => {
    mocks.duplicateAgentProfileAction.mockResolvedValue({
      id: "p-3",
      name: "Alpha Copy",
    } as unknown as AgentProfile);
    renderRows();

    const row = screen.getByLabelText("Alpha").closest('[data-testid="agent-profile-row"]');
    if (!row) throw new Error("no row for Alpha");
    fireEvent.click(row.querySelector('[data-testid="duplicate-profile-p-1"]')!);

    await waitFor(() => expect(mocks.duplicateAgentProfileAction).toHaveBeenCalledWith("p-1"));
  });
});

describe("AgentProfilesSubList layout", () => {
  beforeEach(() => cleanup());
  afterEach(() => cleanup());

  it("renders profile rows without the count or create action", () => {
    render(<AgentProfilesSubList savedAgent={AGENT} agentName="claude" />);

    expect(screen.queryByText("2 profiles", { exact: true })).toBeNull();
    expect(screen.getAllByTestId("agent-profile-row")).toHaveLength(2);
    expect(screen.queryByTestId("new-profile-claude")).toBeNull();
  });

  it("omits the profile body when the agent has no saved profiles", () => {
    render(<AgentProfilesSubList savedAgent={undefined} agentName="claude" />);

    expect(screen.queryByTestId("agent-profiles-claude")).toBeNull();
  });
});
