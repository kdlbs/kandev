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
import { submitOnboarding } from "./setup-wizard";

const mockCompleteOnboarding = vi.mocked(completeOnboarding);
const mockUpdateUserSettings = vi.mocked(updateUserSettings);

const WORKSPACE_NAME = "My Workspace";
const NEW_WORKSPACE_ID = "ws-new-1";

const BASE_WIZARD_DATA = {
  workspaceName: WORKSPACE_NAME,
  taskPrefix: "MY",
  agentName: "CEO",
  agentProfileId: "profile-1",
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
  it("calls completeOnboarding with the wizard data", async () => {
    await submitOnboarding(BASE_WIZARD_DATA);

    expect(mockCompleteOnboarding).toHaveBeenCalledOnce();
    expect(mockCompleteOnboarding).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceName: WORKSPACE_NAME }),
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
