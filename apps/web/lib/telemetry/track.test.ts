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

const SAMPLE_ACTION = "task.create";

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
    trackAction(SAMPLE_ACTION);
    await flushAsync();

    updateTelemetryConsentCache({ status: "granted", env_disabled: true });
    trackAction(SAMPLE_ACTION);
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
    trackAction(SAMPLE_ACTION);
    await flushAsync();

    expect(fetchTelemetryConsent).toHaveBeenCalledTimes(1);
    expect(sendTelemetryEvents).toHaveBeenCalledTimes(2);
    expect(sendTelemetryEvents).toHaveBeenNthCalledWith(1, [
      { name: "ui_page_viewed", properties: { page: "settings.system.telemetry" } },
    ]);
    expect(sendTelemetryEvents).toHaveBeenNthCalledWith(2, [
      { name: "ui_action", properties: { action: SAMPLE_ACTION } },
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

  it("never ends with a separator when the cap lands on one", () => {
    // 63 chars of segment + "/x" → the dot separator falls exactly at the cap
    const result = normalizePathForTelemetry("/" + "a".repeat(63) + "/x");
    expect(result.endsWith(".")).toBe(false);
    expect(/^[a-z0-9][a-z0-9_.:-]*[a-z0-9]$|^[a-z0-9]$/.test(result)).toBe(true);
  });
});
