import { describe, it, expect, vi, beforeEach } from "vitest";
import type { TelemetryConsentState } from "@/lib/types/telemetry";

const fetchTelemetryConsent = vi.fn<() => Promise<TelemetryConsentState>>();
const sendTelemetryEvents = vi.fn<() => Promise<{ accepted: number }>>();

vi.mock("@/lib/api/domains/telemetry-api", () => ({
  fetchTelemetryConsent: (...args: unknown[]) =>
    (fetchTelemetryConsent as unknown as (...a: unknown[]) => unknown)(...args),
  sendTelemetryEvents: (...args: unknown[]) =>
    (sendTelemetryEvents as unknown as (...a: unknown[]) => unknown)(...args),
}));

import {
  normalizePathForTelemetry,
  resetTelemetryTrackingForTests,
  trackAction,
  trackPageView,
  updateTelemetryConsentCache,
} from "./track";

function flushAsync(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

beforeEach(() => {
  resetTelemetryTrackingForTests();
  fetchTelemetryConsent.mockReset();
  sendTelemetryEvents.mockReset();
  sendTelemetryEvents.mockResolvedValue({ accepted: 1 });
});

describe("track gating", () => {
  it("sends nothing while consent is unasked", async () => {
    fetchTelemetryConsent.mockResolvedValue({ status: "unasked", env_disabled: false });
    trackPageView("home");
    await flushAsync();
    expect(sendTelemetryEvents).not.toHaveBeenCalled();
  });

  it("sends nothing when denied or env-disabled", async () => {
    fetchTelemetryConsent.mockResolvedValue({ status: "denied", env_disabled: false });
    trackAction("task.create");
    await flushAsync();

    updateTelemetryConsentCache({ status: "granted", env_disabled: true });
    trackAction("task.create");
    await flushAsync();

    expect(sendTelemetryEvents).not.toHaveBeenCalled();
  });

  it("sends when granted, and fetches consent only once", async () => {
    fetchTelemetryConsent.mockResolvedValue({
      status: "granted",
      install_id: "abc",
      env_disabled: false,
    });
    trackPageView("settings.system.telemetry");
    trackAction("task.create");
    await flushAsync();

    expect(fetchTelemetryConsent).toHaveBeenCalledTimes(1);
    expect(sendTelemetryEvents).toHaveBeenCalledTimes(2);
    expect(sendTelemetryEvents).toHaveBeenCalledWith([
      { name: "ui_page_viewed", properties: { page: "settings.system.telemetry" } },
    ]);
  });

  it("reacts immediately to a consent cache update", async () => {
    fetchTelemetryConsent.mockResolvedValue({ status: "unasked", env_disabled: false });
    updateTelemetryConsentCache({ status: "granted", install_id: "abc", env_disabled: false });
    trackPageView("home");
    await flushAsync();
    expect(fetchTelemetryConsent).not.toHaveBeenCalled();
    expect(sendTelemetryEvents).toHaveBeenCalledTimes(1);
  });

  it("swallows consent fetch failures", async () => {
    fetchTelemetryConsent.mockRejectedValue(new Error("backend down"));
    trackPageView("home");
    await flushAsync();
    expect(sendTelemetryEvents).not.toHaveBeenCalled();
  });
});

describe("normalizePathForTelemetry", () => {
  it("maps the root to home", () => {
    expect(normalizePathForTelemetry("/")).toBe("home");
  });

  it("joins segments with dots", () => {
    expect(normalizePathForTelemetry("/settings/system/telemetry")).toBe(
      "settings.system.telemetry",
    );
  });

  it("collapses UUIDs and numeric ids", () => {
    expect(normalizePathForTelemetry("/t/c2685309-00d9-41e8-bfe7-c29948b31530")).toBe("t.id");
    expect(normalizePathForTelemetry("/tasks/12345")).toBe("tasks.id");
  });

  it("strips unsafe characters and caps length", () => {
    const result = normalizePathForTelemetry("/Settings/Wörk spaces/" + "x".repeat(200));
    expect(result.startsWith("settings.wrkspaces")).toBe(true);
    expect(result.length).toBeLessThanOrEqual(64);
    expect(/^[a-z0-9][a-z0-9_.:-]*$/.test(result)).toBe(true);
  });
});
