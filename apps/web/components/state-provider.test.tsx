import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import {
  readQueuedTaskCreateLastUsedState,
  resetTaskCreateLastUsedSync,
  syncTaskCreateLastUsed,
} from "./task-create-dialog-handlers";
import { StateProvider, useAppStore } from "./state-provider";

function ShowMetricsPreference({ label }: { label: string }) {
  const enabled = useAppStore((state) => state.userSettings.systemMetricsDisplay.showInTopbar);
  return (
    <div>
      {label}:{enabled ? "on" : "off"}
    </div>
  );
}

function EnableMetricsFromNestedProvider() {
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);
  return (
    <button
      type="button"
      onClick={() =>
        setUserSettings({
          ...userSettings,
          systemMetricsDisplay: { showInTopbar: true, simplified: false },
          loaded: true,
        })
      }
    >
      Enable metrics
    </button>
  );
}

beforeEach(() => {
  window.localStorage.clear();
  resetTaskCreateLastUsedSync({ clearQueued: true });
  vi.restoreAllMocks();
});

describe("StateProvider", () => {
  it("reuses the parent store for nested route providers", async () => {
    render(
      <StateProvider>
        <ShowMetricsPreference label="root" />
        <StateProvider
          initialState={{
            userSettings: {
              ...defaultSettingsState.userSettings,
              systemMetricsDisplay: { showInTopbar: false, simplified: false },
              loaded: true,
            },
          }}
        >
          <EnableMetricsFromNestedProvider />
        </StateProvider>
      </StateProvider>,
    );

    expect(screen.getByText("root:off")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Enable metrics" }));
    expect(await screen.findByText("root:on")).toBeTruthy();
  });
});

describe("StateProvider task-create queued overlay", () => {
  it("clears the queued overlay when loaded settings catch up", () => {
    syncTaskCreateLastUsed({ repository_id: "repo-1", branch: "main" });

    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            taskCreateLastUsed: {
              repositoryId: "repo-1",
              branch: "main",
              agentProfileId: null,
              executorProfileId: null,
              workflowIdsByWorkspace: {},
            },
          },
        }}
      >
        <div>ready</div>
      </StateProvider>,
    );

    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });

  it("compares workflow history by content when clearing the queued overlay", () => {
    syncTaskCreateLastUsed({ workspace_id: "workspace-1", workflow_id: "workflow-1" });

    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            taskCreateLastUsed: {
              repositoryId: null,
              branch: null,
              agentProfileId: null,
              executorProfileId: null,
              workflowIdsByWorkspace: { "workspace-1": "workflow-1" },
            },
          },
        }}
      >
        <div>ready</div>
      </StateProvider>,
    );

    expect(readQueuedTaskCreateLastUsedState()).toEqual({});
  });
});
