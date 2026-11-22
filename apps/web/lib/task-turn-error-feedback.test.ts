import { afterEach, describe, expect, it, vi } from "vitest";
import { notifyTurnErrorAcknowledgementFailure } from "./task-turn-error-feedback";

const toastError = vi.hoisted(() => vi.fn());
vi.mock("@/lib/toast/sonner", () => ({ toast: { error: toastError } }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

afterEach(() => vi.clearAllMocks());

describe("notifyTurnErrorAcknowledgementFailure", () => {
  it("shows one localized warning for a failed accepted-prompt acknowledgement", () => {
    notifyTurnErrorAcknowledgementFailure(true);
    expect(toastError).toHaveBeenCalledTimes(1);
    expect(toastError).toHaveBeenCalledWith("task:turnErrorAcknowledgementFailed");
  });

  it("stays quiet for successful or absent acknowledgement", () => {
    notifyTurnErrorAcknowledgementFailure(false);
    notifyTurnErrorAcknowledgementFailure(undefined);
    expect(toastError).not.toHaveBeenCalled();
  });
});
