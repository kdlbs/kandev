import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";

const listUtilityAgents = vi.fn();
const updateUtilityAgent = vi.fn();
const deleteUtilityAgent = vi.fn();
const fetchUserSettings = vi.fn();
const updateUserSettings = vi.fn();

vi.mock("@/lib/api/domains/utility-api", () => ({
  listUtilityAgents: (...args: unknown[]) => listUtilityAgents(...args),
  updateUtilityAgent: (...args: unknown[]) => updateUtilityAgent(...args),
  deleteUtilityAgent: (...args: unknown[]) => deleteUtilityAgent(...args),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: (...args: unknown[]) => fetchUserSettings(...args),
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

vi.mock("@/components/settings/config-chat-agent-section", () => ({
  ConfigChatAgentSection: () => null,
}));

vi.mock("@/components/settings/utility-agent-dialog", () => ({
  UtilityAgentDialog: () => null,
}));

vi.mock("@/components/settings/utility-sections", async () => {
  const actual = await vi.importActual<typeof import("./utility-sections")>("./utility-sections");
  return {
    ...actual,
    DefaultModelSection: ({ onProfileChange }: { onProfileChange: (value: string) => void }) => (
      <button type="button" onClick={() => onProfileChange("profile-1")}>
        Pick default profile
      </button>
    ),
    PerActionOverridesSection: () => null,
    CustomAgentsSection: () => null,
  };
});

import { UtilityAgentsSection } from "./utility-agents-section";

function agent(id: string): UtilityAgent {
  return {
    id,
    name: id,
    description: "",
    builtin: true,
    enabled: true,
    agent_id: "agent-1",
    model: "",
    prompt: "",
    created_at: "",
    updated_at: "",
  };
}

function ReadDefaultUtilityAgentProfileId() {
  const value = useAppStore((state) => state.userSettings.defaultUtilityAgentProfileId);
  return <div data-testid="stored-profile-id">{value ?? ""}</div>;
}

function SetNewerDefaultUtilityAgentProfile() {
  const storeApi = useAppStoreApi();
  return (
    <button
      type="button"
      onClick={() =>
        storeApi.getState().setUserSettings({
          ...storeApi.getState().userSettings,
          defaultUtilityAgentProfileId: "profile-2",
          revision: 2,
        })
      }
    >
      Apply newer profile update
    </button>
  );
}

function renderSection() {
  return render(
    <StateProvider initialState={{ userSettings: { ...defaultState.userSettings } }}>
      <SettingsSaveProvider>
        <UtilityAgentsSection />
      </SettingsSaveProvider>
      <SetNewerDefaultUtilityAgentProfile />
      <ReadDefaultUtilityAgentProfileId />
    </StateProvider>,
  );
}

beforeEach(() => {
  listUtilityAgents.mockReset().mockResolvedValue({ agents: [agent("commit")] });
  fetchUserSettings.mockReset().mockResolvedValue({ settings: {} });
  updateUserSettings
    .mockReset()
    .mockImplementation((payload: Record<string, unknown>) =>
      Promise.resolve({ settings: { revision: 1, ...payload } }),
    );
  updateUtilityAgent.mockReset().mockResolvedValue({});
  deleteUtilityAgent.mockReset().mockResolvedValue({});
});

afterEach(cleanup);

describe("UtilityAgentsSection save", () => {
  it("pushes the saved default utility agent profile id into the store", async () => {
    renderSection();

    fireEvent.click(await screen.findByRole("button", { name: "Pick default profile" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        default_utility_agent_profile_id: "profile-1",
      }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("stored-profile-id").textContent).toBe("profile-1"),
    );
  });

  it("does not overwrite a newer store update when the save response arrives late", async () => {
    let resolveUpdate: (value: unknown) => void = () => undefined;
    updateUserSettings.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      }),
    );

    renderSection();

    fireEvent.click(await screen.findByRole("button", { name: "Pick default profile" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: "Apply newer profile update" }));
    resolveUpdate({
      settings: {
        revision: 1,
        default_utility_agent_profile_id: "profile-1",
      },
    });

    await waitFor(() =>
      expect(screen.getByTestId("stored-profile-id").textContent).toBe("profile-2"),
    );
  });
});
