import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { useJiraTaskPresets } from "./use-task-presets";
import type { JiraStoredPreset } from "./presets";

const STORAGE_KEY = "kandev:jira:task-presets:v1";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));

function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);

const preset: JiraStoredPreset = {
  id: "custom",
  label: "Custom",
  hint: "Do work",
  icon: "code",
  prompt_template: "Work on {{key}}",
};

describe("useJiraTaskPresets", () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: { jira_task_presets: null },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("ignores stale local presets when backend settings are empty", async () => {
    localStorageMock.setItem(STORAGE_KEY, JSON.stringify([preset]));

    const { result } = renderHook(() => useJiraTaskPresets());

    await waitFor(() => expect(result.current.stored).not.toContainEqual(preset));
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});
