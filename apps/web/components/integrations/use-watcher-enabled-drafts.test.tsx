import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { useWatcherEnabledDrafts } from "./use-watcher-enabled-drafts";

const watch = { id: "watch-1", enabled: true };

afterEach(cleanup);

function Harness({ save }: { save: (enabled: boolean) => Promise<void> }) {
  const drafts = useWatcherEnabledDrafts({
    id: "test-watches",
    items: [watch],
    saveEnabled: (_watch, enabled) => save(enabled),
  });
  return (
    <button onClick={() => drafts.toggleEnabled(drafts.items[0])}>
      {drafts.items[0].enabled ? "Disable" : "Enable"}
    </button>
  );
}

describe("useWatcherEnabledDrafts", () => {
  it("returns to a clean state when a watcher is toggled twice", () => {
    const save = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsSaveProvider>
        <Harness save={save} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Disable" }));
    fireEvent.click(screen.getByRole("button", { name: "Enable" }));

    expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull();
    expect(save).not.toHaveBeenCalled();
  });

  it("keeps failed changes dirty and retries them", async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error("offline")).mockResolvedValue(undefined);
    render(
      <SettingsSaveProvider>
        <Harness save={save} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Disable" }));
    expect(save).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Retry save" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry save" }));

    await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save changes" })).toBeNull());
  });
});
