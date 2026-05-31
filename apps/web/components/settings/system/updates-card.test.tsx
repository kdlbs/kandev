import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";
import type { SelfUpdateController } from "@/hooks/domains/system/use-self-update";

const mocks = vi.hoisted(() => ({
  useUpdates: vi.fn(),
  useSelfUpdate: vi.fn(),
}));

vi.mock("@/hooks/domains/system/use-updates", () => ({
  useUpdates: mocks.useUpdates,
}));

vi.mock("@/hooks/domains/system/use-self-update", () => ({
  useSelfUpdate: mocks.useSelfUpdate,
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

describe("UpdatesCard self-update", () => {
  beforeEach(() => {
    mocks.useUpdates.mockReset();
    mocks.useSelfUpdate.mockReset();
    mocks.useSelfUpdate.mockReturnValue(selfUpdate());
  });

  afterEach(() => {
    cleanup();
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
