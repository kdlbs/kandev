import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";
import type { SelfUpdateController } from "@/hooks/domains/system/use-self-update";
import type { DesktopUpdaterController } from "@/hooks/domains/system/use-desktop-updater";
import { requestNavigation } from "@/lib/routing/navigation-guard";
import { SettingsSaveProvider } from "../settings-save-provider";

const mocks = vi.hoisted(() => ({
  useUpdates: vi.fn(),
  useSelfUpdate: vi.fn(),
  useDesktopUpdater: vi.fn(),
}));

vi.mock("@/hooks/domains/system/use-updates", () => ({
  useUpdates: mocks.useUpdates,
}));

vi.mock("@/hooks/domains/system/use-self-update", () => ({
  useSelfUpdate: mocks.useSelfUpdate,
}));

vi.mock("@/hooks/domains/system/use-desktop-updater", () => ({
  useDesktopUpdater: mocks.useDesktopUpdater,
}));

// The @kandev/ui Spinner source trips the classic JSX runtime under vitest;
// stub it so the card (and progress block) can render in jsdom.
vi.mock("@kandev/ui/spinner", () => ({
  Spinner: () => null,
}));

import { UpdatesCard } from "./updates-card";

const APPLY_TESTID = "system-updates-apply";
const ARIA_CHECKED = "aria-checked";
const CHECK_TESTID = "system-updates-check";
const CHANNEL_TESTID = "system-updates-channel";
const ERROR_TESTID = "system-updates-error";
const LATEST_TESTID = "system-updates-latest";
const SETTINGS_DIRTY_ATTRIBUTE = "data-settings-dirty";
const SAVE_CHANGES_NAME = "Save changes";

function updates(overrides: Partial<UpdatesResponse> = {}): UpdatesResponse {
  return {
    current: "v1.0.0",
    latest: "v1.0.1",
    latest_url: "https://example/v1.0.1",
    latest_checked_at: "2026-05-29T00:00:00.000Z",
    update_available: true,
    channel: "stable",
    channel_editable: true,
    channel_unsupported_reason: "",
    install: {
      running_as_service: true,
      managed_service: true,
      mode: "user",
      manager: "systemd",
      kind: "npm",
    },
    apply_supported: true,
    ...overrides,
  };
}

function updatesCard(props: ComponentProps<typeof UpdatesCard> = {}) {
  return (
    <SettingsSaveProvider>
      <UpdatesCard {...props} />
    </SettingsSaveProvider>
  );
}

function renderUpdatesCard(props: ComponentProps<typeof UpdatesCard> = {}) {
  return render(updatesCard(props));
}

function selfUpdate(overrides: Partial<SelfUpdateController> = {}): SelfUpdateController {
  return {
    phase: "idle",
    targetVersion: null,
    errorMessage: null,
    isUpdating: false,
    start: vi.fn().mockResolvedValue(undefined),
    dismiss: vi.fn(),
    ...overrides,
  };
}

function desktopUpdater(
  overrides: Partial<DesktopUpdaterController> = {},
): DesktopUpdaterController {
  return {
    available: false,
    state: null,
    checking: false,
    installing: false,
    error: null,
    check: vi.fn().mockResolvedValue(undefined),
    install: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

beforeEach(() => {
  mocks.useUpdates.mockReset();
  mocks.useSelfUpdate.mockReset();
  mocks.useDesktopUpdater.mockReset();
  mocks.useSelfUpdate.mockReturnValue(selfUpdate());
  mocks.useDesktopUpdater.mockReturnValue(desktopUpdater());
});

afterEach(() => {
  cleanup();
});

describe("UpdatesCard self-update", () => {
  it("uses hook-owned checking state to hide Apply update", () => {
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel: vi.fn(),
      isChecking: true,
      error: null,
    });

    renderUpdatesCard();
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId(CHECK_TESTID).hasAttribute("disabled")).toBe(true);
  });

  it("passes a stable document reload instead of the update-metadata refetch", () => {
    const reloadDocument = vi.fn();
    const reloadMetadataFirst = vi.fn();
    const reloadMetadataSecond = vi.fn();
    mocks.useUpdates
      .mockReturnValueOnce({
        updates: updates(),
        check: vi.fn(),
        reload: reloadMetadataFirst,
      })
      .mockReturnValue({
        updates: updates(),
        check: vi.fn(),
        reload: reloadMetadataSecond,
      });

    const { rerender } = renderUpdatesCard({ reloadDocument });
    const firstCompletion = mocks.useSelfUpdate.mock.calls.at(-1)?.[0]?.onComplete;

    rerender(updatesCard({ reloadDocument }));
    const secondCompletion = mocks.useSelfUpdate.mock.calls.at(-1)?.[0]?.onComplete;

    expect(firstCompletion).toBe(reloadDocument);
    expect(secondCompletion).toBe(reloadDocument);
    firstCompletion?.();
    expect(reloadDocument).toHaveBeenCalledOnce();
    expect(reloadMetadataFirst).not.toHaveBeenCalled();
    expect(reloadMetadataSecond).not.toHaveBeenCalled();
  });

  it("does not render Apply update when the install is not a managed service", () => {
    mocks.useUpdates.mockReturnValue({
      updates: updates({
        install: { running_as_service: false, managed_service: false },
        apply_supported: false,
        apply_unsupported_reason: "Kandev is not running as a managed service.",
        manual_commands: ["kandev service install"],
      }),
      check: vi.fn(),
      reload: vi.fn(),
    });

    renderUpdatesCard();

    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId("system-updates-manual").textContent).toContain(
      "Kandev is not running as a managed service.",
    );
  });

  it("starts the self-update only after confirmation", async () => {
    const start = vi.fn().mockResolvedValue(undefined);
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useSelfUpdate.mockReturnValue(selfUpdate({ start }));

    renderUpdatesCard();
    fireEvent.click(screen.getByTestId(APPLY_TESTID));
    fireEvent.click(await screen.findByTestId("system-updates-apply-confirm"));

    await waitFor(() => expect(start).toHaveBeenCalledTimes(1));
  });

  it("hides the Apply button and shows progress while updating", () => {
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useSelfUpdate.mockReturnValue(
      selfUpdate({ phase: "restarting", targetVersion: "v1.0.1", isUpdating: true }),
    );

    renderUpdatesCard();

    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    const progress = screen.getByTestId("system-updates-progress");
    expect(progress.getAttribute("data-phase")).toBe("restarting");
    expect(progress.textContent).toContain("Restarting Kandev");
  });

  it("shows the updated confirmation when done", () => {
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useSelfUpdate.mockReturnValue(
      selfUpdate({ phase: "done", targetVersion: "v1.0.1", isUpdating: false }),
    );

    renderUpdatesCard();

    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId("system-updates-progress").textContent).toContain(
      "Updated to v1.0.1",
    );
  });
});

describe("UpdatesCard desktop package updates", () => {
  it("shows manual package guidance instead of Apply for non-AppImage Linux installs", () => {
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useDesktopUpdater.mockReturnValue(
      desktopUpdater({
        available: true,
        state: {
          phase: "available",
          currentVersion: "1.0.0",
          latestVersion: "1.1.0",
          releaseNotes: null,
          releaseUrl: "https://example.test/v1.1.0",
          checkedAtEpochMs: 42,
          downloadedBytes: null,
          totalBytes: null,
          installSupported: false,
          installUnsupportedReason:
            "Download the latest package and update it with your package manager.",
          error: null,
        },
      }),
    );

    renderUpdatesCard();

    expect(screen.getByTestId(LATEST_TESTID).textContent).toBe("1.1.0");
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId("system-updates-manual").textContent).toContain("package manager");
  });
});

describe("UpdatesCard desktop updater", () => {
  it("uses native desktop update state without changing the responsive action layout", () => {
    mocks.useUpdates.mockReturnValue({
      updates: updates({ update_available: false }),
      check: vi.fn(),
      reload: vi.fn(),
    });
    mocks.useDesktopUpdater.mockReturnValue(
      desktopUpdater({
        available: true,
        state: {
          phase: "available",
          currentVersion: "1.0.0",
          latestVersion: "1.1.0",
          releaseNotes: "Changes",
          releaseUrl: "https://example.test/v1.1.0",
          checkedAtEpochMs: 42,
          downloadedBytes: null,
          totalBytes: null,
          installSupported: true,
          installUnsupportedReason: null,
          error: null,
        },
      }),
    );

    renderUpdatesCard();

    expect(screen.getByTestId("system-updates-current").textContent).toBe("1.0.0");
    expect(screen.getByTestId(LATEST_TESTID).textContent).toBe("1.1.0");
    expect(screen.getByTestId(APPLY_TESTID)).toBeTruthy();
    expect(screen.queryByTestId("system-updates-manual")).toBeNull();
    expect(screen.getByTestId("system-updates-actions").className).toContain("flex-col");
  });

  it("starts a desktop update only after confirmation", async () => {
    const install = vi.fn().mockResolvedValue(undefined);
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useDesktopUpdater.mockReturnValue(
      desktopUpdater({
        available: true,
        install,
        state: {
          phase: "available",
          currentVersion: "1.0.0",
          latestVersion: "1.1.0",
          releaseNotes: null,
          releaseUrl: null,
          checkedAtEpochMs: null,
          downloadedBytes: null,
          totalBytes: null,
          installSupported: true,
          installUnsupportedReason: null,
          error: null,
        },
      }),
    );

    renderUpdatesCard();
    fireEvent.click(screen.getByTestId(APPLY_TESTID));
    expect(install).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByTestId("system-updates-apply-confirm"));

    await waitFor(() => expect(install).toHaveBeenCalledOnce());
  });

  it("shows native download progress and errors", () => {
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useDesktopUpdater.mockReturnValue(
      desktopUpdater({
        available: true,
        installing: true,
        error: "Signature verification failed",
        state: {
          phase: "downloading",
          currentVersion: "1.0.0",
          latestVersion: "1.1.0",
          releaseNotes: null,
          releaseUrl: null,
          checkedAtEpochMs: 42,
          downloadedBytes: 25,
          totalBytes: 100,
          installSupported: true,
          installUnsupportedReason: null,
          error: "Signature verification failed",
        },
      }),
    );

    renderUpdatesCard();

    expect(screen.getByTestId("system-updates-progress").textContent).toContain("25 of 100 bytes");
    expect(screen.getByTestId(ERROR_TESTID).textContent).toContain("Signature verification failed");
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
  });
});

describe("UpdatesCard channel setting", () => {
  it("keeps Nightly local until the shared settings action saves it", async () => {
    const saveChannel = vi.fn().mockResolvedValue(
      updates({
        channel: "nightly",
        latest: "1.0.1-nightly.shaabcdef123456",
        latest_url: "https://www.npmjs.com/package/kandev/v/1.0.1-nightly.shaabcdef123456",
      }),
    );
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel,
      error: null,
    });

    renderUpdatesCard();
    const stable = screen.getByRole("radio", { name: /^Stable/ });
    const nightly = screen.getByRole("radio", { name: /^Nightly/ });

    expect(stable.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(nightly.getAttribute(ARIA_CHECKED)).toBe("false");
    fireEvent.click(nightly);

    expect(nightly.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(saveChannel).not.toHaveBeenCalled();
    expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId(LATEST_TESTID).textContent).toBe("-");
    expect(screen.getByTestId("system-updates-checked-at").textContent).toBe("Last checked never");
    expect(screen.getByTestId("system-updates-channel-nightly").textContent).toContain(
      "Install-wide",
    );
    expect(screen.getByTestId("system-updates-channel-nightly").textContent).toContain(
      "exact version shown",
    );
    expect(screen.getByTestId(CHECK_TESTID).hasAttribute("disabled")).toBe(true);
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.queryByTestId("system-updates-release-link")).toBeNull();
    expect(screen.getByTestId("system-updates-channel-pending").textContent).toContain(
      "Save channel changes",
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));

    await waitFor(() => expect(saveChannel).toHaveBeenCalledWith("nightly"));
    await waitFor(() =>
      expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe(
        "false",
      ),
    );
  });
});

describe("UpdatesCard channel save errors", () => {
  it("clears a previous check error after a channel save succeeds", async () => {
    let updatesError: string | null = null;
    const check = vi.fn(async () => {
      updatesError = "npm registry unavailable";
      throw new Error(updatesError);
    });
    const saveChannel = vi.fn(async () => {
      updatesError = null;
      return updates({ channel: "nightly" });
    });
    mocks.useUpdates.mockImplementation(() => ({
      updates: updates(),
      check,
      reload: vi.fn(),
      saveChannel,
      isChecking: false,
      error: updatesError,
    }));

    const { rerender } = renderUpdatesCard();
    fireEvent.click(screen.getByTestId(CHECK_TESTID));
    await waitFor(() => expect(check).toHaveBeenCalledOnce());
    rerender(updatesCard());
    expect(screen.getByTestId(ERROR_TESTID).textContent).toContain("npm registry unavailable");

    fireEvent.click(screen.getByRole("radio", { name: /^Nightly/ }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));

    await waitFor(() => expect(saveChannel).toHaveBeenCalledWith("nightly"));
    rerender(updatesCard());
    expect(screen.queryByTestId(ERROR_TESTID)).toBeNull();
  });

  it("keeps a failed channel save dirty and retryable", async () => {
    const saveChannel = vi
      .fn()
      .mockRejectedValueOnce(new Error("npm registry unavailable"))
      .mockResolvedValueOnce(updates({ channel: "nightly" }));
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel,
      error: null,
    });

    renderUpdatesCard();
    const nightly = screen.getByRole("radio", { name: /^Nightly/ });
    fireEvent.click(nightly);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(nightly.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Retry save" }));

    await waitFor(() => expect(saveChannel).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe(
        "false",
      ),
    );
  });

  it("replaces a previous check error with the channel save failure", async () => {
    let updatesError: string | null = null;
    const check = vi.fn(async () => {
      updatesError = "retry after 27 seconds";
      throw new Error(updatesError);
    });
    const saveChannel = vi.fn(async () => {
      updatesError = "channel save failed";
      throw new Error(updatesError);
    });
    mocks.useUpdates.mockImplementation(() => ({
      updates: updates(),
      check,
      reload: vi.fn(),
      saveChannel,
      isChecking: false,
      error: updatesError,
    }));

    const { rerender } = renderUpdatesCard();
    fireEvent.click(screen.getByTestId(CHECK_TESTID));
    await waitFor(() => expect(check).toHaveBeenCalledOnce());
    rerender(updatesCard());
    expect(screen.getByTestId(ERROR_TESTID).textContent).toContain("27s");

    fireEvent.click(screen.getByRole("radio", { name: /^Nightly/ }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));

    await waitFor(() => expect(screen.getByText("Couldn't save")).toBeTruthy());
    expect(screen.getByTestId(ERROR_TESTID).textContent).toContain("channel save failed");
    expect(screen.getByTestId(ERROR_TESTID).textContent).not.toContain("27s");
  });
});

describe("UpdatesCard concurrent channel edits", () => {
  it("keeps update actions blocked when the draft reverts during a save", async () => {
    let resolveSave!: (value: UpdatesResponse) => void;
    const saveChannel = vi.fn(
      () =>
        new Promise<UpdatesResponse>((resolve) => {
          resolveSave = resolve;
        }),
    );
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel,
      error: null,
    });

    renderUpdatesCard();
    const stable = screen.getByRole("radio", { name: /^Stable/ });
    fireEvent.click(screen.getByRole("radio", { name: /^Nightly/ }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));
    await waitFor(() => expect(saveChannel).toHaveBeenCalledWith("nightly"));

    fireEvent.click(stable);

    expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe("false");
    expect(screen.getByTestId("system-updates-card").getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe(
      "true",
    );
    expect(screen.getByTestId(CHECK_TESTID).hasAttribute("disabled")).toBe(true);
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId("system-updates-channel-pending").textContent).toContain("Saving");

    const proceed = vi.fn();
    act(() => requestNavigation(proceed));
    expect(proceed).not.toHaveBeenCalled();
    expect(await screen.findByRole("alertdialog")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Discard and leave" }).hasAttribute("disabled")).toBe(
      true,
    );

    await act(async () => {
      resolveSave(updates({ channel: "nightly" }));
    });
    await waitFor(() =>
      expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe(
        "true",
      ),
    );
    expect(proceed).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Continue editing" }));
  });

  it("preserves a newer channel choice while an earlier save is pending", async () => {
    let resolveSave!: (value: UpdatesResponse) => void;
    const saveChannel = vi.fn(
      () =>
        new Promise<UpdatesResponse>((resolve) => {
          resolveSave = resolve;
        }),
    );
    let authoritative = updates();
    mocks.useUpdates.mockImplementation(() => ({
      updates: authoritative,
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel,
      error: null,
    }));

    const { rerender } = renderUpdatesCard();
    const stable = screen.getByRole("radio", { name: /^Stable/ });
    const nightly = screen.getByRole("radio", { name: /^Nightly/ });
    fireEvent.click(nightly);
    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_NAME }));
    await waitFor(() => expect(saveChannel).toHaveBeenCalledWith("nightly"));

    fireEvent.click(stable);
    authoritative = updates({ channel: "nightly" });
    rerender(updatesCard());
    expect(stable.getAttribute(ARIA_CHECKED)).toBe("true");

    await act(async () => {
      resolveSave(authoritative);
    });

    expect(stable.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(screen.getByTestId(CHANNEL_TESTID).getAttribute(SETTINGS_DIRTY_ATTRIBUTE)).toBe("true");
  });
});

describe("UpdatesCard channel navigation", () => {
  it("discards an unsaved Nightly choice through the shared navigation guard", async () => {
    const saveChannel = vi.fn();
    const proceed = vi.fn();
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel,
      error: null,
    });

    renderUpdatesCard();
    const stable = screen.getByRole("radio", { name: /^Stable/ });
    const nightly = screen.getByRole("radio", { name: /^Nightly/ });
    fireEvent.click(nightly);
    await screen.findByRole("button", { name: SAVE_CHANGES_NAME });

    act(() => requestNavigation(proceed));
    fireEvent.click(await screen.findByRole("button", { name: "Discard and leave" }));

    await waitFor(() => expect(stable.getAttribute(ARIA_CHECKED)).toBe("true"));
    expect(saveChannel).not.toHaveBeenCalled();
    expect(proceed).toHaveBeenCalledOnce();
  });
});

describe("UpdatesCard channel availability", () => {
  it("disables Nightly and renders the server capability reason", () => {
    mocks.useUpdates.mockReturnValue({
      updates: updates({
        channel_editable: false,
        channel_unsupported_reason:
          "Nightly updates require a Kandev-managed npm or npx user service.",
      }),
      check: vi.fn(),
      reload: vi.fn(),
      saveChannel: vi.fn(),
      error: null,
    });

    renderUpdatesCard();

    const stable = screen.getByRole("radio", { name: /^Stable/ });
    const nightly = screen.getByRole("radio", { name: /^Nightly/ });
    const reasonId = screen.getByTestId("system-updates-channel-reason").id;
    expect(stable.getAttribute(ARIA_CHECKED)).toBe("true");
    expect(stable.hasAttribute("disabled")).toBe(true);
    expect(nightly.hasAttribute("disabled")).toBe(true);
    expect(stable.getAttribute("aria-describedby")).toContain(reasonId);
    expect(nightly.getAttribute("aria-describedby")).toContain(reasonId);
    expect(screen.getByTestId("system-updates-channel-reason").textContent).toContain(
      "Kandev-managed npm or npx user service",
    );
  });

  it("does not expose an npm channel selector in the Desktop updater", () => {
    mocks.useDesktopUpdater.mockReturnValue(
      desktopUpdater({
        available: true,
        state: {
          phase: "up-to-date",
          currentVersion: "1.0.0",
          latestVersion: "1.0.0",
          releaseNotes: null,
          releaseUrl: null,
          checkedAtEpochMs: 42,
          downloadedBytes: null,
          totalBytes: null,
          installSupported: true,
          installUnsupportedReason: null,
          error: null,
        },
      }),
    );

    renderUpdatesCard();

    expect(screen.queryByTestId(CHANNEL_TESTID)).toBeNull();
    expect(mocks.useUpdates).not.toHaveBeenCalled();
    expect(mocks.useSelfUpdate).not.toHaveBeenCalled();
  });
});
