import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import { AgentConfigurationTab } from "./agent-configuration-tab";

// Mock toast so the act-like hooks don't error and we don't need the toast
// provider tree for these isolated tests.
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    updateAgentProfile: vi.fn().mockResolvedValue({}),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const AGENT_TIMESTAMP = "2026-05-04T00:00:00Z";
const CLAUDE_AGENT_ID = "claude-code";
const WORKSPACE_ID = "ws-1";
const WORKER_ID = "agent-worker";
const MANAGER_ID = "agent-manager";
const SAVE_BUTTON = "Save Configuration";
const REPORTS_TO_COMBOBOX = "Reports to";

const baseAgent = {
  id: "agent-ceo",
  workspaceId: WORKSPACE_ID,
  name: "CEO",
  role: "ceo",
  status: "idle",
  agentProfileId: "profile-claude-1",
  createdAt: AGENT_TIMESTAMP,
  updatedAt: AGENT_TIMESTAMP,
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 5000,
  maxConcurrentSessions: 2,
} as AgentProfile;

const PROFILE_OPTION = {
  id: "profile-claude-1",
  label: "Claude • Default",
  agent_id: CLAUDE_AGENT_ID,
  agent_name: CLAUDE_AGENT_ID,
  cli_passthrough: false,
};

function renderConfigTab(agents: AgentProfile[], agent: AgentProfile) {
  render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: {
          ...defaultOfficeState.office,
          agentProfilesByWorkspaceId: { [WORKSPACE_ID]: agents },
        },
        agentProfiles: { items: [PROFILE_OPTION], version: 0 },
      }}
    >
      <AgentConfigurationTab agent={agent} />
    </StateProvider>,
  );
}

describe("AgentConfigurationTab", () => {
  it("reconciles the form with the canonical profile returned by the backend", async () => {
    vi.mocked(updateAgentProfile).mockResolvedValueOnce({
      ...baseAgent,
      agentProfileId: toAgentProfileId(baseAgent.id),
      name: "Canonical CEO",
    });
    renderConfigTab([baseAgent], baseAgent);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Local edit" } });
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON }));

    await waitFor(() => {
      expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Canonical CEO");
    });
  });

  it("shows create-agent capability for CEO agents", () => {
    renderConfigTab([baseAgent], baseAgent);

    expect(screen.getByTestId("agent-capability-preview").textContent).toContain("Create agent");
  });

  it("omits create-agent capability for default worker agents", () => {
    const worker = {
      ...baseAgent,
      id: toAgentProfileId(WORKER_ID),
      name: "Worker",
      role: "worker" as const,
    };
    renderConfigTab([worker], worker);

    expect(screen.getByTestId("agent-capability-preview").textContent).not.toContain(
      "Create agent",
    );
  });

  it("PATCHes reportsTo when a new manager is selected", async () => {
    const manager = { ...baseAgent, id: MANAGER_ID, name: "Manager" } as AgentProfile;
    const worker = {
      ...baseAgent,
      id: WORKER_ID,
      name: "Worker",
      role: "worker" as const,
      reportsTo: undefined,
    } as AgentProfile;
    renderConfigTab([manager, worker], worker);

    fireEvent.click(screen.getByRole("combobox", { name: REPORTS_TO_COMBOBOX }));
    fireEvent.click(await screen.findByRole("option", { name: "Manager" }));
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON }));

    await waitFor(() => {
      expect(updateAgentProfile).toHaveBeenCalledWith(
        WORKER_ID,
        expect.objectContaining({ reportsTo: MANAGER_ID }),
      );
    });
  });

  it("PATCHes reportsTo as an empty string when cleared to None", async () => {
    const worker = {
      ...baseAgent,
      id: WORKER_ID,
      name: "Worker",
      role: "worker" as const,
      reportsTo: "agent-ceo",
    } as AgentProfile;
    renderConfigTab([baseAgent, worker], worker);

    fireEvent.click(screen.getByRole("combobox", { name: REPORTS_TO_COMBOBOX }));
    fireEvent.click(await screen.findByRole("option", { name: "None" }));
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON }));

    await waitFor(() => {
      expect(updateAgentProfile).toHaveBeenCalledWith(
        WORKER_ID,
        expect.objectContaining({ reportsTo: "" }),
      );
    });
  });

  it("does not offer the agent itself as a reports-to option", async () => {
    const manager = { ...baseAgent, id: MANAGER_ID, name: "Manager" } as AgentProfile;
    renderConfigTab([manager], manager);

    fireEvent.click(screen.getByRole("combobox", { name: REPORTS_TO_COMBOBOX }));
    await waitFor(() => {
      expect(screen.queryByRole("option", { name: "Manager" })).toBeNull();
    });
  });

  it("does not offer a descendant as a reports-to option", async () => {
    const manager = { ...baseAgent, id: MANAGER_ID, name: "Manager" } as AgentProfile;
    const worker = {
      ...baseAgent,
      id: WORKER_ID,
      name: "Worker",
      role: "worker" as const,
      reportsTo: MANAGER_ID,
    } as AgentProfile;
    renderConfigTab([manager, worker], manager);

    fireEvent.click(screen.getByRole("combobox", { name: REPORTS_TO_COMBOBOX }));
    await waitFor(() => {
      expect(screen.queryByRole("option", { name: "Worker" })).toBeNull();
    });
  });
});
