import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { Agent, AgentProfile } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  deleteAgentProfileAction: vi.fn(),
  duplicateAgentProfileAction: vi.fn(),
  toast: vi.fn(),
  routerPush: vi.fn(),
  responsive: { isFullDesktop: false, isFinePointer: false },
}));

function profile(id: string, name: string): AgentProfile {
  return { id, name, agentDisplayName: "Claude Code" } as AgentProfile;
}

const ALPHA_PROFILE_NAME = "Alpha";
const PROFILE_ROW_SELECTOR = '[data-testid="agent-profile-row"]';
const PROFILE_ACTIONS_MENU_SELECTOR = '[data-testid="profile-actions-menu-p-1"]';

const AGENT = {
  id: "agent-1",
  name: "claude",
  profiles: [profile("p-1", ALPHA_PROFILE_NAME), profile("p-2", "Beta")],
} as unknown as Agent;

const EMPTY_AGENT = { ...AGENT, profiles: [] } as unknown as Agent;

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

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));

vi.mock("@/components/routing/app-link", () => ({
  default: ({ children, ...props }: { children?: ReactNode }) => <a {...props}>{children}</a>,
}));

// Radix's dropdown does not open under jsdom clicks, so keep menu items inline
// and reachable while exercising the real local confirmation shell.
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

import { AgentProfilesSubList, ProfileRow } from "./agent-profiles-section";

function renderWithTooltipProvider(ui: ReactNode) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

function renderRows() {
  return renderWithTooltipProvider(
    <>
      {AGENT.profiles.map((p) => (
        <ProfileRow key={p.id} agent={AGENT} profile={p} />
      ))}
    </>,
  );
}

function confirmDeleteFor(name: string) {
  const row = screen.getByLabelText(name).closest(PROFILE_ROW_SELECTOR);
  if (!row) throw new Error(`no row for ${name}`);
  const profileId = AGENT.profiles.find((p) => p.name === name)?.id;
  if (!profileId) throw new Error(`no profile id for ${name}`);
  fireEvent.click(row.querySelector(`[data-testid="delete-profile-${profileId}"]`)!);
  fireEvent.click(row.querySelector('[data-testid="agent-profile-delete-confirm"]')!);
}

describe("ProfileRow deletion", () => {
  beforeEach(() => {
    mocks.responsive.isFullDesktop = false;
    mocks.responsive.isFinePointer = false;
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

  it("keeps simple confirmation inline on coarse pointers and lets cancel be a no-op", async () => {
    mocks.deleteAgentProfileAction.mockResolvedValue({ status: "ok" });
    renderRows();

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    fireEvent.click(row.querySelector('[data-testid="delete-profile-p-1"]')!);

    expect(
      row.querySelector('[data-testid="agent-profile-delete-inline-confirmation"]'),
    ).not.toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();

    const inlineConfirmation = row.querySelector(
      '[data-testid="agent-profile-delete-inline-confirmation"]',
    );
    if (!inlineConfirmation) throw new Error("no inline confirmation");
    fireEvent.click(
      within(inlineConfirmation as HTMLElement).getByRole("button", { name: "Cancel" }),
    );

    expect(mocks.deleteAgentProfileAction).not.toHaveBeenCalled();
    expect(
      row.querySelector('[data-testid="agent-profile-delete-inline-confirmation"]'),
    ).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(row.querySelector(PROFILE_ACTIONS_MENU_SELECTOR)),
    );
  });

  it("restores focus to the row action after deletion fails", async () => {
    mocks.deleteAgentProfileAction.mockResolvedValue({ status: "error", message: "Delete failed" });
    renderRows();

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    if (!row.querySelector(PROFILE_ACTIONS_MENU_SELECTOR)) {
      throw new Error("no profile actions trigger");
    }
    fireEvent.click(row.querySelector('[data-testid="delete-profile-p-1"]')!);
    fireEvent.click(row.querySelector('[data-testid="agent-profile-delete-confirm"]')!);

    await waitFor(() => expect(mocks.deleteAgentProfileAction).toHaveBeenCalledWith("p-1"));
    await waitFor(() =>
      expect(document.activeElement).toBe(row.querySelector(PROFILE_ACTIONS_MENU_SELECTOR)),
    );
  });
});

describe("ProfileRow duplicate", () => {
  beforeEach(() => {
    mocks.responsive.isFullDesktop = false;
    storeState = {
      settingsAgents: { items: [{ ...AGENT, profiles: [...AGENT.profiles] }] },
      agentProfiles: { items: [] },
    };
  });
  afterEach(() => cleanup());

  it("calls the duplicate action with the profile id", async () => {
    mocks.duplicateAgentProfileAction.mockResolvedValue({
      id: "p-3",
      name: "Alpha Copy",
    } as unknown as AgentProfile);
    renderRows();

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    fireEvent.click(row.querySelector('[data-testid="duplicate-profile-p-1"]')!);

    await waitFor(() => expect(mocks.duplicateAgentProfileAction).toHaveBeenCalledWith("p-1"));
  });
});

describe("ProfileRow responsive actions", () => {
  beforeEach(() => {
    storeState = {
      settingsAgents: { items: [{ ...AGENT, profiles: [...AGENT.profiles] }] },
      agentProfiles: { items: [] },
    };
    mocks.responsive.isFullDesktop = false;
    mocks.responsive.isFinePointer = false;
    vi.clearAllMocks();
  });
  afterEach(() => cleanup());

  it("renders compact icon-only duplicate and delete actions inline on full desktop", async () => {
    mocks.responsive.isFullDesktop = true;
    renderWithTooltipProvider(<ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />);

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    expect(row.querySelector('[data-testid="profile-actions-inline-p-1"]')).not.toBeNull();
    const duplicate = row.querySelector<HTMLButtonElement>(
      '[data-testid="duplicate-profile-inline-p-1"]',
    );
    const deleteButton = row.querySelector<HTMLButtonElement>(
      '[data-testid="delete-profile-inline-p-1"]',
    );
    expect(duplicate).not.toBeNull();
    expect(deleteButton).not.toBeNull();
    expect(duplicate?.textContent).toBe("");
    expect(deleteButton?.textContent).toBe("");
    expect(duplicate?.getAttribute("data-size")).toBe("icon");
    expect(deleteButton?.getAttribute("data-size")).toBe("icon");
    expect(row.querySelector(PROFILE_ACTIONS_MENU_SELECTOR)).toBeNull();

    fireEvent.focus(duplicate!);
    await waitFor(() => expect(screen.getByRole("tooltip", { name: "Duplicate" })).toBeTruthy());
    fireEvent.focus(deleteButton!);
    await waitFor(() => expect(screen.getByRole("tooltip", { name: "Delete" })).toBeTruthy());
  });

  it("keeps duplicate and delete in the overflow menu below full desktop", () => {
    renderWithTooltipProvider(<ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />);

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    expect(row.querySelector('[data-testid="profile-actions-inline-p-1"]')).toBeNull();
    expect(row.querySelector(PROFILE_ACTIONS_MENU_SELECTOR)).not.toBeNull();
    expect(row.querySelector('[data-testid="duplicate-profile-p-1"]')).not.toBeNull();
    expect(row.querySelector('[data-testid="delete-profile-p-1"]')).not.toBeNull();
  });

  it("wires inline duplicate and delete to the shared row behavior", async () => {
    mocks.responsive.isFullDesktop = true;
    mocks.duplicateAgentProfileAction.mockResolvedValue({
      id: "p-3",
      name: "Alpha Copy",
    } as unknown as AgentProfile);
    mocks.deleteAgentProfileAction.mockResolvedValue({ status: "ok" });
    renderWithTooltipProvider(<ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />);

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    fireEvent.click(row.querySelector('[data-testid="duplicate-profile-inline-p-1"]')!);
    fireEvent.click(row.querySelector('[data-testid="delete-profile-inline-p-1"]')!);
    fireEvent.click(row.querySelector('[data-testid="agent-profile-delete-confirm"]')!);

    await waitFor(() => expect(mocks.duplicateAgentProfileAction).toHaveBeenCalledWith("p-1"));
    await waitFor(() => expect(mocks.deleteAgentProfileAction).toHaveBeenCalledWith("p-1"));
  });

  it("anchors simple confirmation to the row action on fine pointers", () => {
    mocks.responsive.isFullDesktop = true;
    mocks.responsive.isFinePointer = true;
    renderWithTooltipProvider(<ProfileRow agent={AGENT} profile={AGENT.profiles[0]} />);

    const row = screen.getByLabelText(ALPHA_PROFILE_NAME).closest(PROFILE_ROW_SELECTOR);
    if (!row) throw new Error(`no row for ${ALPHA_PROFILE_NAME}`);
    fireEvent.click(row.querySelector('[data-testid="delete-profile-inline-p-1"]')!);

    expect(screen.getByTestId("agent-profile-delete-confirm-popover")).toBeTruthy();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});

describe("AgentProfilesSubList layout", () => {
  beforeEach(() => cleanup());
  afterEach(() => cleanup());

  it("renders profile rows without the count or create action", () => {
    renderWithTooltipProvider(<AgentProfilesSubList savedAgent={AGENT} agentName="claude" />);

    expect(screen.queryByText("2 profiles", { exact: true })).toBeNull();
    expect(screen.getAllByTestId("agent-profile-row")).toHaveLength(2);
    expect(screen.queryByTestId("new-profile-claude")).toBeNull();
  });

  it("omits the profile body when no agent record exists", () => {
    renderWithTooltipProvider(<AgentProfilesSubList savedAgent={undefined} agentName="claude" />);

    expect(screen.queryByTestId("agent-profiles-claude")).toBeNull();
  });

  it("omits the profile body when the saved agent has no profiles", () => {
    renderWithTooltipProvider(<AgentProfilesSubList savedAgent={EMPTY_AGENT} agentName="claude" />);

    expect(screen.queryByTestId("agent-profiles-claude")).toBeNull();
  });
});
