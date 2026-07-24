import { describe, expect, it } from "vitest";
import { isMessageSendError, MessageSendError } from "./message-send-error";

describe("isMessageSendError", () => {
  it("classifies session-unavailable so submission shows a deterministic not-sent error", () => {
    const error = new MessageSendError(
      "session-unavailable",
      "The selected session is not available for input.",
    );

    expect(isMessageSendError(error)).toBe(true);
    expect(error.message).toBe("The selected session is not available for input.");
  });

  it("does not classify an arbitrary transport error as deterministically not sent", () => {
    expect(isMessageSendError(new Error("timeout"))).toBe(false);
  });
});
