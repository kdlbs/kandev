import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AgentProfile, OfficeMeta } from "@/lib/state/slices/office/types";
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
vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getAgentRoute: vi.fn(async () => ({ overrides: undefined, provider_order: [] })),
  getProviderHealth: vi.fn(async () => ({ health: [] })),
  getRoutingPreview: vi.fn(async () => ({ previews: [] })),
  getWorkspaceRouting: vi.fn(async () => ({ config: null, known_providers: [] })),
}));

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

function officeMeta(): OfficeMeta {
  return {
    statuses: [],
    priorities: [],
    roles: [
      { id: "ceo", label: "CEO", description: "Coordinator", color: "bg-purple-100" },
      { id: "worker", label: "Worker", description: "Worker", color: "bg-blue-100" },
    ],
    executorTypes: [{ id: "local_pc", label: "Local", description: "Local executor" }],
    skillSourceTypes: [],
    projectStatuses: [],
    agentStatuses: [{ id: "idle", label: "Idle", color: "bg-neutral-400" }],
    routineRunStatuses: [],
    inboxItemTypes: [],
    permissions: [],
    permissionDefaults: {
      ceo: { create_agent: true },
      worker: { create_agent: false },
    },
  };
}

function renderConfigurationTab(agent: AgentProfile) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.office.meta(), officeMeta());
  queryClient.setQueryData(qk.settings.agents(), {
    agents: [
      {
        id: CLAUDE_AGENT_ID,
        name: CLAUDE_AGENT_ID,
        profiles: [
          {
            id: PROFILE_OPTION.id,
            name: "Default",
            agentId: CLAUDE_AGENT_ID,
            agentDisplayName: "Claude",
            cliPassthrough: false,
            createdAt: AGENT_TIMESTAMP,
            updatedAt: AGENT_TIMESTAMP,
          },
        ],
      },
    ],
    total: 1,
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StateProvider
        initialState={{
          workspaces: { activeId: "ws-1" },
          office: { ...defaultOfficeState.office, agentProfiles: [agent] },
        }}
      >
        <AgentConfigurationTab agent={agent} />
      </StateProvider>
    </QueryClientProvider>,
  );
}

describe("AgentConfigurationTab", () => {
  it("renders the CLI configuration card with the linked profile summary", () => {
    renderConfigurationTab(baseAgent);

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
    renderConfigurationTab(orphan);

    expect(screen.queryByText(/no cli profile selected/i)).toBeNull();
    expect(screen.getByText("Claude")).toBeTruthy();
  });

  it("shows create-agent capability for CEO agents", () => {
    renderConfigurationTab(baseAgent);

    expect(screen.getByTestId("agent-capability-preview").textContent).toContain("Create agent");
  });

  it("omits create-agent capability for default worker agents", () => {
    const worker = {
      ...baseAgent,
      id: toAgentProfileId("agent-worker"),
      name: "Worker",
      role: "worker" as const,
    };
    renderConfigurationTab(worker);

    expect(screen.getByTestId("agent-capability-preview").textContent).not.toContain(
      "Create agent",
    );
  });
});
