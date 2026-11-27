import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { CreateProfilePanel } from "./agent-profile-setup-controls";

vi.mock("@kandev/ui/button", () => ({ Button: () => null }));
vi.mock("@/components/task-create-dialog-options", () => ({ useAgentProfileOptions: vi.fn() }));

vi.mock("@/components/agent/cli-profile-editor", () => ({
  CliProfileEditor: ({ onSaved }: { onSaved: (profile: unknown) => void }) => (
    <button onClick={() => onSaved({ id: "new", agentId: "agent", name: "Created" })}>
      save profile
    </button>
  ),
}));

const option = (id: string) => ({
  id,
  label: id,
  agent_id: "agent",
  agent_name: "Agent",
  cli_passthrough: false,
});

describe("office profile setup ordering", () => {
  afterEach(() => vi.clearAllMocks());

  it("places a newly saved profile first in the flat list and existing settings agent", async () => {
    let store!: StoreApi<AppState>;
    function Capture() {
      store = useAppStoreApi();
      return null;
    }
    const onProfileSaved = vi.fn();
    render(
      <StateProvider>
        <Capture />
        <CreateProfilePanel
          settingsAgents={[{ id: "agent", name: "Agent" }]}
          wizardProfiles={[option("old")] as never}
          canCancel
          setAgentProfiles={(profiles) => store.getState().setAgentProfiles(profiles)}
          onProfileSaved={onProfileSaved}
          onClose={() => {}}
        />
      </StateProvider>,
    );
    const api = store;
    act(() => {
      api.getState().setSettingsAgents([
        {
          id: "agent",
          name: "Agent",
          profiles: [{ id: "old", name: "Old" }],
        } as never,
      ]);
      api
        .getState()
        .setAgentProfiles([
          option("old"),
          { ...option("office"), workspace_id: "office-workspace" },
        ] as never);
    });
    fireEvent.click(screen.getByText("save profile"));
    await waitFor(() =>
      expect(api.getState().agentProfiles.items.map((item) => item.id)).toEqual([
        "new",
        "old",
        "office",
      ]),
    );
    expect(api.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "new",
      "old",
    ]);
    expect(onProfileSaved).toHaveBeenCalledWith("new");
  });
});
