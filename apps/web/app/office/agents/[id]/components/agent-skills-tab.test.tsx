import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AgentProfile, Skill } from "@/lib/state/slices/office/types";
import { AgentSkillsTab } from "./agent-skills-tab";

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
const CHECKBOX_STATE_ATTRIBUTE = "data-state";

const baseAgent = {
  id: "agent-ceo",
  workspaceId: "ws-1",
  name: "CEO",
  role: "ceo",
  status: "idle",
  createdAt: AGENT_TIMESTAMP,
  updatedAt: AGENT_TIMESTAMP,
  permissions: {},
  budgetMonthlyCents: 5000,
  maxConcurrentSessions: 2,
} as AgentProfile;

const baseSkill = {
  id: "skill-protocol",
  workspaceId: "ws-1",
  name: "Kandev Protocol",
  slug: "kandev-protocol",
  sourceType: "system",
  isSystem: true,
  systemVersion: "test",
  defaultForRoles: ["ceo"],
  createdAt: AGENT_TIMESTAMP,
  updatedAt: AGENT_TIMESTAMP,
} satisfies Skill;

function renderSkillsTab(agent: AgentProfile, skills: Skill[] = [baseSkill]) {
  const queryClient = makeQueryClient();
  queryClient.setQueryDefaults(qk.office.skills("ws-1"), { staleTime: Infinity });
  queryClient.setQueryData(qk.office.skills("ws-1"), { skills });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={{ workspaces: { activeId: "ws-1" } }}>
        <TooltipProvider>
          <AgentSkillsTab agent={agent} />
        </TooltipProvider>
      </StateProvider>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

function protocolCheckbox() {
  return screen.getByTestId("skill-toggle-checkbox-kandev-protocol");
}

async function expectProtocolCheckboxState(state: "checked" | "unchecked") {
  await waitFor(() =>
    expect(protocolCheckbox().getAttribute(CHECKBOX_STATE_ATTRIBUTE)).toBe(state),
  );
}

describe("AgentSkillsTab", () => {
  it("checks skills resolved from legacy desiredSkills slugs", async () => {
    renderSkillsTab({ ...baseAgent, skillIds: [], desiredSkills: ["kandev-protocol"] });

    await expectProtocolCheckboxState("checked");
  });

  it("preserves explicitly cleared default skills", async () => {
    renderSkillsTab({ ...baseAgent, skillIds: [], desiredSkills: [] });

    await expectProtocolCheckboxState("unchecked");
  });

  it("falls back to role defaults only when skill fields are absent", async () => {
    renderSkillsTab({ ...baseAgent, skillIds: undefined, desiredSkills: undefined });

    await expectProtocolCheckboxState("checked");
  });

  it("resyncs checked state when the agent query updates after initial render", async () => {
    const skillWithoutRoleDefault = { ...baseSkill, defaultForRoles: [] };
    const { queryClient, rerender } = renderSkillsTab(
      { ...baseAgent, skillIds: [], desiredSkills: [] },
      [skillWithoutRoleDefault],
    );
    expect(protocolCheckbox().getAttribute(CHECKBOX_STATE_ATTRIBUTE)).toBe("unchecked");

    rerender(
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={{ workspaces: { activeId: "ws-1" } }}>
          <TooltipProvider>
            <AgentSkillsTab agent={{ ...baseAgent, skillIds: ["skill-protocol"] }} />
          </TooltipProvider>
        </StateProvider>
      </QueryClientProvider>,
    );

    await expectProtocolCheckboxState("checked");
  });
});
