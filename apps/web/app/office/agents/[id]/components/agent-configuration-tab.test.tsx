import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
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
  it("renders the CLI configuration card with the linked profile summary", () => {
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

    expect(screen.getByText("CLI Configuration")).toBeTruthy();
    // Linked profile is surfaced with the CLI client badge.
    expect(screen.getByText(CLAUDE_AGENT_ID)).toBeTruthy();
  });

  it("uses the office agent row as the CLI profile when no legacy profile link exists", () => {
    const orphan = {
      ...baseAgent,
      agentProfileId: undefined,
      agentId: CLAUDE_AGENT_ID,
      agentDisplayName: "Claude",
    };
    render(
      <StateProvider
        initialState={{
          workspaces: { activeId: "ws-1", items: [] },
          office: { ...defaultOfficeState.office, agentProfiles: [orphan] },
          agentProfiles: { items: [PROFILE_OPTION], version: 0 },
        }}
      >
        <AgentConfigurationTab agent={orphan} />
      </StateProvider>,
    );

    expect(screen.queryByText(/no cli profile selected/i)).toBeNull();
    expect(screen.getByText("Claude")).toBeTruthy();
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
