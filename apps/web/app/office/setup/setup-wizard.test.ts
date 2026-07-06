import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/api/domains/office-api", () => ({
  completeOnboarding: vi.fn(),
  importFromFS: vi.fn(),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(),
}));

import { completeOnboarding } from "@/lib/api/domains/office-api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  DEFAULT_ONBOARDING_TASK_DESCRIPTION,
  DEFAULT_ONBOARDING_TASK_TITLE,
  getInitialData,
  submitOnboarding,
} from "./setup-wizard";

const mockCompleteOnboarding = vi.mocked(completeOnboarding);
const mockUpdateUserSettings = vi.mocked(updateUserSettings);

const WORKSPACE_NAME = "My Workspace";
const NEW_WORKSPACE_ID = "ws-new-1";

const BASE_WIZARD_DATA = {
  workspaceName: WORKSPACE_NAME,
  taskPrefix: "MY",
  agentName: "CEO",
  agentProfileId: "profile-1",
  tierProfileIds: {
    frontier: "profile-frontier",
    balanced: "profile-balanced",
    economy: "profile-economy",
  },
  executorPreference: "local_pc",
  taskTitle: "",
  taskDescription: "",
} as const;

beforeEach(() => {
  mockCompleteOnboarding.mockReset();
  mockUpdateUserSettings.mockReset();
  mockCompleteOnboarding.mockResolvedValue({
    workspaceId: NEW_WORKSPACE_ID,
    agentId: "agent-1",
    projectId: "proj-1",
  } as never);
});

describe("submitOnboarding", () => {
  it("starts new workspaces with a CEO setup task brief", () => {
    const data = getInitialData(WORKSPACE_NAME, "profile-1");

    expect(data.taskTitle).toBe(DEFAULT_ONBOARDING_TASK_TITLE);
    expect(data.taskDescription).toBe(DEFAULT_ONBOARDING_TASK_DESCRIPTION);
    expect(data.taskDescription).toContain("https://github.com/org/repo");
    expect(data.taskDescription).toContain("Create one project per repository");
    expect(data.taskDescription).toContain("Create the agent team");
    expect(data.tierProfileIds).toEqual({
      frontier: "profile-1",
      balanced: "profile-1",
      economy: "profile-1",
    });
  });

  it("calls completeOnboarding with the wizard data", async () => {
    await submitOnboarding(BASE_WIZARD_DATA);

    expect(mockCompleteOnboarding).toHaveBeenCalledOnce();
    expect(mockCompleteOnboarding).toHaveBeenCalledWith(
      expect.objectContaining({
        workspaceName: WORKSPACE_NAME,
        tier_profiles: {
          frontier: "profile-frontier",
          balanced: "profile-balanced",
          economy: "profile-economy",
        },
      }),
    );
  });

  it("does NOT call updateUserSettings after completing onboarding", async () => {
    await submitOnboarding(BASE_WIZARD_DATA);

    expect(mockUpdateUserSettings).not.toHaveBeenCalled();
  });

  it("returns the result from completeOnboarding", async () => {
    const result = await submitOnboarding(BASE_WIZARD_DATA);

    expect(result.workspaceId).toBe(NEW_WORKSPACE_ID);
  });
});
