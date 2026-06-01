import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import type { AgentProfile as OfficeAgent } from "@/lib/state/slices/office/types";
import { AgentConfigurationTab } from "./agent-configuration-tab";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";

const WS_ID = "ws-1";

function renderWithProfiles(
  ui: React.ReactElement,
  profileOptions: unknown[],
  officeAgents: OfficeAgent[] = [],
) {
  const client = createTestQueryClient();
  client.setQueryData(qk.settings.agentProfiles(), profileOptions);
  // AgentConfigurationTab reads workspace agents from TanStack Query
  // (officeQueryOptions.agents), not the (removed) Zustand office mirror.
  client.setQueryData(qk.office.agents(WS_ID), officeAgents);
  return render(
    <QueryClientProvider client={client}>
      <StateProvider initialState={{ workspaces: { activeId: WS_ID } }}>{ui}</StateProvider>
    </QueryClientProvider>,
  );
}

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
    renderWithProfiles(<AgentConfigurationTab agent={baseAgent} />, [PROFILE_OPTION], [baseAgent]);

    expect(screen.getByText("CLI Configuration")).toBeTruthy();
    expect(screen.getByText(CLAUDE_AGENT_ID)).toBeTruthy();
  });

  it("uses the office agent row as the CLI profile when no legacy profile link exists", () => {
    const orphan = {
      ...baseAgent,
      agentProfileId: undefined,
      agentId: CLAUDE_AGENT_ID,
      agentDisplayName: "Claude",
    };
    renderWithProfiles(<AgentConfigurationTab agent={orphan} />, [PROFILE_OPTION], [orphan]);

    expect(screen.queryByText(/no cli profile selected/i)).toBeNull();
    expect(screen.getByText("Claude")).toBeTruthy();
  });

  it("shows create-agent capability for CEO agents", () => {
    renderWithProfiles(<AgentConfigurationTab agent={baseAgent} />, [PROFILE_OPTION], [baseAgent]);

    expect(screen.getByTestId("agent-capability-preview").textContent).toContain("Create agent");
  });

  it("omits create-agent capability for default worker agents", () => {
    const worker = {
      ...baseAgent,
      id: toAgentProfileId("agent-worker"),
      name: "Worker",
      role: "worker" as const,
    };
    renderWithProfiles(<AgentConfigurationTab agent={worker} />, [PROFILE_OPTION], [worker]);

    expect(screen.getByTestId("agent-capability-preview").textContent).not.toContain(
      "Create agent",
    );
  });
});
