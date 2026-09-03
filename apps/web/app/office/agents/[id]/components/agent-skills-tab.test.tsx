import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { listSkills, updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile, Skill } from "@/lib/state/slices/office/types";
import { defaultOfficeState } from "@/lib/state/slices/office/office-slice";
import { AgentSkillsTab } from "./agent-skills-tab";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    listSkills: vi.fn(),
    updateAgentProfile: vi.fn().mockResolvedValue({}),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKSPACE_ID = "ws-1";
const AGENT_TIMESTAMP = "2026-05-04T00:00:00Z";
const CHECKED = "checked";
const DATA_STATE = "data-state";

const skill: Skill = {
  id: "skill-1",
  workspaceId: WORKSPACE_ID,
  name: "Kandev Protocol",
  slug: "kandev-protocol",
  sourceType: "system",
  isSystem: true,
  createdAt: AGENT_TIMESTAMP,
  updatedAt: AGENT_TIMESTAMP,
};

const agentDesiredOnly = {
  id: "agent-1",
  workspaceId: WORKSPACE_ID,
  name: "Analyst",
  role: "worker",
  status: "idle",
  agentProfileId: "profile-1",
  createdAt: AGENT_TIMESTAMP,
  updatedAt: AGENT_TIMESTAMP,
  permissions: {},
  pauseReason: "",
  budgetMonthlyCents: 0,
  maxConcurrentSessions: 1,
  skillIds: [] as string[],
  desiredSkills: [skill.slug],
} as AgentProfile;

function renderSkillsTab(agent: AgentProfile, skills: Skill[]) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { activeId: WORKSPACE_ID, items: [] },
        office: {
          ...defaultOfficeState.office,
          agentProfilesByWorkspaceId: { [WORKSPACE_ID]: [agent] },
          skills,
        },
      }}
    >
      <TooltipProvider>
        <AgentSkillsTab agent={agent} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("AgentSkillsTab union seeding", () => {
  it("checks a skill present only in desiredSkills", async () => {
    vi.mocked(listSkills).mockResolvedValue({ skills: [skill] });
    renderSkillsTab(agentDesiredOnly, [skill]);

    await waitFor(() => {
      const checkbox = screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`);
      expect(checkbox.getAttribute(DATA_STATE)).toBe(CHECKED);
    });
  });

  it("checks the skill once the catalog hydrates after mount", async () => {
    vi.mocked(listSkills).mockResolvedValue({ skills: [skill] });
    // office.skills starts empty: the union has nothing to resolve
    // agent.desiredSkills against until useHydrateSkills's effect resolves.
    renderSkillsTab(agentDesiredOnly, []);

    await waitFor(() => {
      const checkbox = screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`);
      expect(checkbox.getAttribute(DATA_STATE)).toBe(CHECKED);
    });
  });
});

describe("AgentSkillsTab save", () => {
  it("sends both skillIds and desiredSkills, converging the columns", async () => {
    const otherSkill: Skill = { ...skill, id: "skill-2", slug: "other-skill", name: "Other" };
    vi.mocked(listSkills).mockResolvedValue({ skills: [skill, otherSkill] });
    renderSkillsTab(agentDesiredOnly, [skill, otherSkill]);

    await waitFor(() => {
      expect(
        screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`).getAttribute(DATA_STATE),
      ).toBe(CHECKED);
    });

    fireEvent.click(screen.getByTestId(`skill-toggle-checkbox-${otherSkill.slug}`));
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(updateAgentProfile).toHaveBeenCalledWith(
        agentDesiredOnly.id,
        expect.objectContaining({
          skillIds: expect.arrayContaining([skill.id, otherSkill.id]),
          desiredSkills: expect.arrayContaining([skill.slug, otherSkill.slug]),
        }),
      );
    });
  });

  it("removes an unchecked desired-only skill's slug from the saved payload", async () => {
    vi.mocked(listSkills).mockResolvedValue({ skills: [skill] });
    renderSkillsTab(agentDesiredOnly, [skill]);

    await waitFor(() => {
      expect(
        screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`).getAttribute(DATA_STATE),
      ).toBe(CHECKED);
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`));
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(updateAgentProfile).toHaveBeenCalledWith(
        agentDesiredOnly.id,
        expect.objectContaining({ skillIds: [], desiredSkills: [] }),
      );
    });
  });

  it("preserves a desired slug the catalog cannot resolve when an unrelated skill is toggled", async () => {
    const otherSkill: Skill = { ...skill, id: "skill-2", slug: "other-skill", name: "Other" };
    const unresolvedSlug = "cross-workspace-only";
    const agentWithUnresolvedSlug = {
      ...agentDesiredOnly,
      desiredSkills: [skill.slug, unresolvedSlug],
    } as AgentProfile;
    vi.mocked(listSkills).mockResolvedValue({ skills: [skill, otherSkill] });
    renderSkillsTab(agentWithUnresolvedSlug, [skill, otherSkill]);

    await waitFor(() => {
      expect(
        screen.getByTestId(`skill-toggle-checkbox-${skill.slug}`).getAttribute(DATA_STATE),
      ).toBe(CHECKED);
    });

    fireEvent.click(screen.getByTestId(`skill-toggle-checkbox-${otherSkill.slug}`));
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(updateAgentProfile).toHaveBeenCalledWith(
        agentWithUnresolvedSlug.id,
        expect.objectContaining({
          desiredSkills: expect.arrayContaining([skill.slug, otherSkill.slug, unresolvedSlug]),
        }),
      );
    });
  });
});
