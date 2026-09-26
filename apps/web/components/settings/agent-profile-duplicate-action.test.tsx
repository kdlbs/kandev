import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { useProfileDuplicateAction } from "./agent-profile-duplicate-action";

type ToastArgs = {
  title?: string;
  description?: string;
  variant?: "default" | "success" | "error" | "loading";
};

const duplicateAction = vi.fn<(id: string) => Promise<AgentProfile>>();
const saveAll = vi.fn<() => Promise<{ canLeave: boolean; failedIds: Set<string> }>>();
const cancelPendingNavigation = vi.fn<() => void>();
const routerPush = vi.fn<(href: string) => void>();
const toast = vi.fn<(args: ToastArgs) => void>();
const setState = vi.fn<() => void>();

const hoisted = vi.hoisted(() => ({
  coordinatorInvalidReason: undefined as string | undefined,
}));

vi.mock("@/app/actions/agents", () => ({
  duplicateAgentProfileAction: (id: string) => duplicateAction(id),
}));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => ({ setState }) }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveCoordinator: () => ({
    saveAll,
    invalidReason: hoisted.coordinatorInvalidReason,
    cancelPendingNavigation,
  }),
}));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: routerPush }) }));
vi.mock("@/lib/routing/navigation-guard", () => ({
  runWithNavigationBlockerBypassed: (fn: () => void) => fn(),
}));
vi.mock("@/components/settings/agent-profile-page-state", () => ({
  errorMessage: (error: unknown) => (error instanceof Error ? error.message : String(error)),
}));

/** Builds an Agent fixture for the duplicate-action harness. */
function agent(): Agent {
  return { id: "a1", name: "mock-agent", profiles: [] } as unknown as Agent;
}

type DraftOverrides = Partial<Omit<AgentProfile, "id" | "agentId" | "name">> & {
  id?: string;
  agentId?: string;
  name?: string;
};

/** Builds a profile draft (SettingsSaveContributor) for the duplicate-action harness. */
function draft(overrides: DraftOverrides = {}): AgentProfile {
  return {
    id: "p1",
    agentId: "a1",
    name: "Default",
    agentDisplayName: "Mock",
    model: "mock-fast",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    enabled: true,
    createdAt: "",
    updatedAt: "",
    ...overrides,
  } as unknown as AgentProfile;
}

/** Renders the duplicate action with the settings save provider and navigation guard. */
function Harness({ profileDraft = draft() }: { profileDraft?: AgentProfile }) {
  const { handleDuplicate, duplicating } = useProfileDuplicateAction({
    agent: agent(),
    draft: profileDraft,
    modelConfigResolutionPending: false,
    translate: (key: string) => key,
  });
  return (
    <button type="button" onClick={() => void handleDuplicate()} aria-busy={duplicating}>
      duplicate
    </button>
  );
}

/** Reads the most recently pushed toast from the harness. */
function lastToast(): ToastArgs {
  const call = toast.mock.calls.at(-1);
  return call ? call[0] : {};
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  hoisted.coordinatorInvalidReason = undefined;
});

describe("useProfileDuplicateAction", () => {
  it("aborts without POSTing when a contributor cannot save", async () => {
    saveAll.mockResolvedValue({ canLeave: false, failedIds: new Set() });
    render(<Harness />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    });

    expect(duplicateAction).not.toHaveBeenCalled();
    expect(routerPush).not.toHaveBeenCalled();
  });

  it("toasts the coordinator invalid reason when the profile draft is valid but MCP is not", async () => {
    hoisted.coordinatorInvalidReason = "MCP server URL invalid";
    saveAll.mockResolvedValue({ canLeave: false, failedIds: new Set() });
    render(<Harness />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    });

    expect(duplicateAction).not.toHaveBeenCalled();
    expect(lastToast().description).toBe("MCP server URL invalid");
  });

  it("toasts the profile-name validation reason when the draft name is empty", async () => {
    saveAll.mockResolvedValue({ canLeave: false, failedIds: new Set() });
    render(<Harness profileDraft={draft({ name: "  " })} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    });

    expect(duplicateAction).not.toHaveBeenCalled();
    expect(lastToast().description).toBe("agents:profileNameRequired");
  });

  it("falls back to a generic toast when the save is blocked without a specific reason", async () => {
    // canLeave=false with no invalidReason and a valid draft: another save is
    // in flight or newer changes appeared. The abort must still be visible.
    saveAll.mockResolvedValue({ canLeave: false, failedIds: new Set() });
    render(<Harness />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    });

    expect(duplicateAction).not.toHaveBeenCalled();
    expect(lastToast().description).toBe("agents:duplicateSaveBlocked");
  });

  it("POSTs, merges, toasts, and SPA-navigates when the save succeeds", async () => {
    saveAll.mockResolvedValue({ canLeave: true, failedIds: new Set() });
    duplicateAction.mockResolvedValue(draft({ id: "p2", name: "Default Copy" }));
    render(<Harness />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    });

    expect(duplicateAction).toHaveBeenCalledTimes(1);
    expect(duplicateAction).toHaveBeenCalledWith("p1");
    expect(setState).toHaveBeenCalledTimes(1);
    expect(cancelPendingNavigation).toHaveBeenCalledTimes(1);
    expect(routerPush).toHaveBeenCalledWith("/settings/agents/mock-agent/profiles/p2");
    expect(toast.mock.calls.some(([t]) => t?.title === "agents:duplicateProfileSuccess")).toBe(
      true,
    );
  });

  it("ignores a second click while the first duplicate is in flight", async () => {
    saveAll.mockResolvedValue({ canLeave: true, failedIds: new Set() });
    let resolveDuplicate: (value: AgentProfile) => void = () => undefined;
    duplicateAction.mockImplementation(
      () =>
        new Promise<AgentProfile>((resolve) => {
          resolveDuplicate = resolve;
        }),
    );
    render(<Harness />);

    fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    fireEvent.click(screen.getByRole("button", { name: "duplicate" }));
    await act(async () => {
      resolveDuplicate(draft({ id: "p2", name: "Default Copy" }));
    });

    expect(duplicateAction).toHaveBeenCalledTimes(1);
  });
});
