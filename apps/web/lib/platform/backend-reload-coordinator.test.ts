import { describe, expect, it, vi } from "vitest";
import { createBackendReloadCoordinator } from "./backend-reload-coordinator";

describe("backend reload coordinator", () => {
  it("latches the first signal and reports it once", () => {
    const report = vi.fn();
    const coordinator = createBackendReloadCoordinator({ reporter: report });
    const listener = vi.fn();
    coordinator.subscribe(listener);

    expect(coordinator.getSnapshot()).toEqual({
      reloadRequired: false,
      source: null,
      ownerCount: 0,
    });
    expect(coordinator.signal("boot_id_changed")).toBe(true);
    expect(coordinator.signal("settings_interlock_rejected")).toBe(false);

    expect(coordinator.getSnapshot()).toEqual({
      reloadRequired: true,
      source: "boot_id_changed",
      ownerCount: 0,
    });
    expect(listener).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenCalledOnce();
    expect(report).toHaveBeenCalledWith("boot_id_changed");
  });

  it("reports a latched signal when the reporter is installed later", () => {
    const coordinator = createBackendReloadCoordinator();
    const report = vi.fn();

    coordinator.signal("settings_interlock_rejected");
    coordinator.setDiagnosticReporter(report);
    coordinator.setDiagnosticReporter(report);

    expect(report).toHaveBeenCalledOnce();
    expect(report).toHaveBeenCalledWith("settings_interlock_rejected");
  });

  it("tracks idempotent intentional restart owners", () => {
    const coordinator = createBackendReloadCoordinator();
    const firstRelease = coordinator.registerOwner();
    const secondRelease = coordinator.registerOwner();

    expect(coordinator.getSnapshot().ownerCount).toBe(2);
    firstRelease();
    firstRelease();
    expect(coordinator.getSnapshot().ownerCount).toBe(1);
    secondRelease();
    expect(coordinator.getSnapshot().ownerCount).toBe(0);
    secondRelease();
    expect(coordinator.getSnapshot().ownerCount).toBe(0);
  });
});
