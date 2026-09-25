import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";

import { ApiError } from "@/lib/api/client";
import { isRoutineNotFiringError, routineNotFiringMessage } from "./routine-not-firing";

const FALLBACK_KEY = "office:failedToRunRoutine";

function notFiringError(status: string): ApiError {
  return new ApiError("routine cannot fire", 409, {
    error: "routine cannot fire",
    error_code: "routine_not_firing",
    status,
  });
}

describe("isRoutineNotFiringError", () => {
  it("recognizes the status-refusal error code", () => {
    expect(isRoutineNotFiringError(notFiringError("paused"))).toBe(true);
  });

  it("does not recognize an unrelated ApiError", () => {
    const other = new ApiError("boom", 500, { error: "boom" });
    expect(isRoutineNotFiringError(other)).toBe(false);
  });

  it("does not recognize a plain Error", () => {
    expect(isRoutineNotFiringError(new Error("boom"))).toBe(false);
  });
});

describe("routineNotFiringMessage", () => {
  const translateWithStatusLabels = (key: string, options?: Record<string, unknown>): string => {
    if (key === "office:routineStatusPaused") return "Pausada";
    if (key === "office:routineStatusArchived") return "Arquivada";
    if (key === "office:routineNotFiring") {
      return `Cannot run: routine status is ${String(options?.status ?? "")}`;
    }
    return key;
  };

  it.each([
    ["paused", "Pausada"],
    ["archived", "Arquivada"],
  ])("uses the translated label for %s", (status, translatedStatus) => {
    expect(
      routineNotFiringMessage(notFiringError(status), translateWithStatusLabels, FALLBACK_KEY),
    ).toBe(`Cannot run: routine status is ${translatedStatus}`);
  });

  it("keeps an unknown status as diagnostic text", () => {
    expect(
      routineNotFiringMessage(notFiringError("on_hold"), translateWithStatusLabels, FALLBACK_KEY),
    ).toBe("Cannot run: routine status is on_hold");
  });

  it("renders localized copy naming the observed status", () => {
    const msg = routineNotFiringMessage(notFiringError("paused"), t, FALLBACK_KEY);
    expect(msg).not.toBe(t(FALLBACK_KEY));
    expect(msg).toContain("Paused");
  });

  it("falls back to the server message for any other Error", () => {
    const err = new Error("some other failure");
    expect(routineNotFiringMessage(err, t, FALLBACK_KEY)).toBe("some other failure");
  });

  it("falls back to the fallback key when there is no message", () => {
    expect(routineNotFiringMessage("not an error", t, FALLBACK_KEY)).toBe(t(FALLBACK_KEY));
  });
});
