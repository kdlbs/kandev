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

const baseAgent = {
  id: "agent-ceo",
  workspaceId: "ws-1",
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

describe("AgentConfigurationTab", () => {
  it("reconciles the form with the canonical profile returned by the backend", async () => {
    vi.mocked(updateAgentProfile).mockResolvedValueOnce({
      ...baseAgent,
      agentProfileId: toAgentProfileId(baseAgent.id),
      name: "Canonical CEO",
    });
    render(
      <StateProvider
        initialState={{
          workspaces: { activeId: "ws-1", items: [] },
          office: { ...defaultOfficeState.office, agentProfiles: [baseAgent] },
          agentProfiles: { items: [PROFILE_OPTION], version: 0 },
        }}
      >
        <AgentConfigurationTab agent={baseAgent} />
      </StateProvider>,
    );

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Local edit" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Configuration" }));

    await waitFor(() => {
      expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Canonical CEO");
    });
  });

  it("shows create-agent capability for CEO agents", () => {
    render(
      <StateProvider
        initialState={{
          workspaces: { activeId: "ws-1", items: [] },
          office: { ...defaultOfficeState.office, agentProfiles: [baseAgent] },
          agentProfiles: { items: [PROFILE_OPTION], version: 0 },
        }}
      >
        <AgentConfigurationTab agent={baseAgent} />
      </StateProvider>,
    );

    expect(screen.getByTestId("agent-capability-preview").textContent).toContain("Create agent");
  });

  it("omits create-agent capability for default worker agents", () => {
    const worker = {
      ...baseAgent,
      id: toAgentProfileId("agent-worker"),
      name: "Worker",
      role: "worker" as const,
    };
    render(
      <StateProvider
        initialState={{
          workspaces: { activeId: "ws-1", items: [] },
          office: { ...defaultOfficeState.office, agentProfiles: [worker] },
          agentProfiles: { items: [PROFILE_OPTION], version: 0 },
        }}
      >
        <AgentConfigurationTab agent={worker} />
      </StateProvider>,
    );

    expect(screen.getByTestId("agent-capability-preview").textContent).not.toContain(
      "Create agent",
    );
  });
});
