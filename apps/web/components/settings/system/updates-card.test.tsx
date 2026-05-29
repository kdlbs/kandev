import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  useUpdates: vi.fn(),
}));

vi.mock("@/hooks/domains/system/use-updates", () => ({
  useUpdates: mocks.useUpdates,
}));

vi.mock("./job-progress-indicator", () => ({
  JobProgressIndicator: () => null,
}));

import { UpdatesCard } from "./updates-card";

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

describe("UpdatesCard self-update gate", () => {
  beforeEach(() => {
    mocks.useUpdates.mockReset();
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
      apply: vi.fn(),
      isApplying: false,
    });

    render(<UpdatesCard />);

    expect(screen.queryByTestId("system-updates-apply")).toBeNull();
    expect(screen.getByTestId("system-updates-manual").textContent).toContain(
      "Kandev is not running as a managed service.",
    );
  });

  it("calls apply only after confirmation when the service install is supported", async () => {
    const apply = vi.fn().mockResolvedValue({ job_id: "job-1" });
    mocks.useUpdates.mockReturnValue({
      updates: updates(),
      check: vi.fn(),
      apply,
      isApplying: false,
    });

    render(<UpdatesCard />);
    fireEvent.click(screen.getByTestId("system-updates-apply"));
    fireEvent.click(await screen.findByTestId("system-updates-apply-confirm"));

    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
  });
});
