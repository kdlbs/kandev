import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { STORAGE_KEYS } from "@/lib/settings/constants";
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
          systemMetricsDisplay: { showInTopbar: true },
          loaded: true,
        })
      }
    >
      Enable metrics
    </button>
  );
}

function ObserveLastUsedCache({ onSeen }: { onSeen: (value: string | null) => void }) {
  useEffect(() => {
    onSeen(window.localStorage.getItem(STORAGE_KEYS.LAST_REPOSITORY_ID));
  }, [onSeen]);
  return null;
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
              systemMetricsDisplay: { showInTopbar: false },
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

describe("StateProvider task-create cache", () => {
  it("syncs task-create last-used settings to localStorage without redundant writes", async () => {
    function UpdateUnrelatedSetting() {
      const setUserSettings = useAppStore((state) => state.setUserSettings);
      const userSettings = useAppStore((state) => state.userSettings);
      return (
        <button
          type="button"
          onClick={() =>
            setUserSettings({
              ...userSettings,
              showReleaseNotification: !userSettings.showReleaseNotification,
            })
          }
        >
          Toggle unrelated
        </button>
      );
    }

    const setItemSpy = vi.spyOn(Storage.prototype, "setItem");
    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            taskCreateLastUsed: {
              repositoryId: "repo-1",
              branch: "main",
              agentProfileId: "agent-1",
              executorProfileId: "exec-1",
            },
          },
        }}
      >
        <UpdateUnrelatedSetting />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(window.localStorage.getItem(STORAGE_KEYS.LAST_REPOSITORY_ID)).toBe(
        JSON.stringify("repo-1"),
      );
    });
    const writesAfterInitialSync = setItemSpy.mock.calls.length;

    fireEvent.click(screen.getByRole("button", { name: "Toggle unrelated" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Toggle unrelated" })).toBeTruthy();
    });
    expect(setItemSpy).toHaveBeenCalledTimes(writesAfterInitialSync);
  });

  it("primes task-create last-used localStorage before child effects run", () => {
    const onSeen = vi.fn();
    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            loaded: true,
            taskCreateLastUsed: {
              repositoryId: "repo-1",
              branch: "main",
              agentProfileId: "agent-1",
              executorProfileId: "exec-1",
            },
          },
        }}
      >
        <ObserveLastUsedCache onSeen={onSeen} />
      </StateProvider>,
    );

    expect(onSeen).toHaveBeenCalledWith(JSON.stringify("repo-1"));
  });
});

describe("StateProvider task-create cache clearing", () => {
  it("clears cached task-create choices when loaded settings have no replacement", () => {
    window.localStorage.setItem(STORAGE_KEYS.LAST_REPOSITORY_ID, JSON.stringify("repo-cached"));
    window.localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("feature-cached"));
    window.localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("agent-cached"));
    window.localStorage.setItem(
      STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID,
      JSON.stringify("exec-cached"),
    );

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
            },
          },
        }}
      >
        <div>ready</div>
      </StateProvider>,
    );

    expect(window.localStorage.getItem(STORAGE_KEYS.LAST_REPOSITORY_ID)).toBeNull();
    expect(window.localStorage.getItem(STORAGE_KEYS.LAST_BRANCH)).toBeNull();
    expect(window.localStorage.getItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID)).toBeNull();
    expect(window.localStorage.getItem(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID)).toBeNull();
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
