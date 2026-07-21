import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";
import type { SelfUpdateController } from "@/hooks/domains/system/use-self-update";
import type { DesktopUpdaterController } from "@/hooks/domains/system/use-desktop-updater";

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

function updates(overrides: Partial<UpdatesResponse> = {}): UpdatesResponse {
  return {
    current: "v1.0.0",
    latest: "v1.0.1",
    latest_url: "https://example/v1.0.1",
    latest_checked_at: "2026-05-29T00:00:00.000Z",
    update_available: true,
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

    render(<UpdatesCard />);

    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
    expect(screen.getByTestId("system-updates-manual").textContent).toContain(
      "Kandev is not running as a managed service.",
    );
  });

  it("starts the self-update only after confirmation", async () => {
    const start = vi.fn().mockResolvedValue(undefined);
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useSelfUpdate.mockReturnValue(selfUpdate({ start }));

    render(<UpdatesCard />);
    fireEvent.click(screen.getByTestId(APPLY_TESTID));
    fireEvent.click(await screen.findByTestId("system-updates-apply-confirm"));

    await waitFor(() => expect(start).toHaveBeenCalledTimes(1));
  });

  it("hides the Apply button and shows progress while updating", () => {
    mocks.useUpdates.mockReturnValue({ updates: updates(), check: vi.fn(), reload: vi.fn() });
    mocks.useSelfUpdate.mockReturnValue(
      selfUpdate({ phase: "restarting", targetVersion: "v1.0.1", isUpdating: true }),
    );

    render(<UpdatesCard />);

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

    render(<UpdatesCard />);

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

    render(<UpdatesCard />);

    expect(screen.getByTestId("system-updates-latest").textContent).toBe("1.1.0");
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

    render(<UpdatesCard />);

    expect(screen.getByTestId("system-updates-current").textContent).toBe("1.0.0");
    expect(screen.getByTestId("system-updates-latest").textContent).toBe("1.1.0");
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

    render(<UpdatesCard />);
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

    render(<UpdatesCard />);

    expect(screen.getByTestId("system-updates-progress").textContent).toContain("25 of 100 bytes");
    expect(screen.getByTestId("system-updates-error").textContent).toContain(
      "Signature verification failed",
    );
    expect(screen.queryByTestId(APPLY_TESTID)).toBeNull();
  });
});
