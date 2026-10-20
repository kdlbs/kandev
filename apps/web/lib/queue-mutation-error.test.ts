import { describe, expect, it } from "vitest";
import { QueueAdmissionError } from "./api/domains/queue-api";
import { queueMutationErrorKey } from "./queue-mutation-error";

describe("queueMutationErrorKey", () => {
  it("uses retry guidance for admission backpressure", () => {
    expect(
      queueMutationErrorKey(
        new QueueAdmissionError("unavailable"),
        "chat:failedToClearQueuedMessages",
      ),
    ).toBe("task:queueAdmissionUnavailable");
  });

  it("keeps the operation feedback for other failures", () => {
    expect(
      queueMutationErrorKey(new Error("storage failure"), "chat:failedToClearQueuedMessages"),
    ).toBe("chat:failedToClearQueuedMessages");
    expect(
      queueMutationErrorKey(new QueueAdmissionError("validation"), "chat:queueEditSaveFailed"),
    ).toBe("chat:queueEditSaveFailed");
  });
});
