import { beforeEach, describe, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { createQueuedUserSettingsSync } from "./user-settings-sync";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("createQueuedUserSettingsSync", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(updateUserSettings).mockReset();
  });

  it("serializes backend writes so later payloads are sent after earlier ones finish", async () => {
    const first = deferred<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings)
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
    const sync = createQueuedUserSettingsSync<string>((value) => ({
      preferred_shell: value,
    }));

    void sync("bash");
    void sync("zsh");

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledTimes(1);
    });
    expect(updateUserSettings).toHaveBeenCalledWith({ preferred_shell: "bash" });

    first.resolve({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenCalledTimes(2);
      expect(updateUserSettings).toHaveBeenLastCalledWith({ preferred_shell: "zsh" });
    });
  });

  it("retries a transient settings write before processing the next payload", async () => {
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>)
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);
    const sync = createQueuedUserSettingsSync<string>((value) => ({
      preferred_shell: value,
    }));

    void sync("bash");
    void sync("zsh");

    await waitFor(() => {
      expect(updateUserSettings).toHaveBeenNthCalledWith(1, { preferred_shell: "bash" });
      expect(updateUserSettings).toHaveBeenNthCalledWith(2, { preferred_shell: "bash" });
      expect(updateUserSettings).toHaveBeenNthCalledWith(3, { preferred_shell: "zsh" });
    });
  });
});
