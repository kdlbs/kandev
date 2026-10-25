import { describe, expect, it } from "vitest";
import { isDetachedManagedConversation } from "./retained-managed-conversation";

describe("isDetachedManagedConversation", () => {
  it("recognizes only host-retained transcripts detached from an installation", () => {
    expect(
      isDetachedManagedConversation({
        metadata: { "kandev.managed_retained": true, "kandev.detached": true },
      } as never),
    ).toBe(true);
    expect(
      isDetachedManagedConversation({
        metadata: { "kandev.managed_retained": true, "kandev.detached": false },
      } as never),
    ).toBe(false);
    expect(isDetachedManagedConversation({ metadata: { "kandev.detached": true } } as never)).toBe(
      false,
    );
    expect(isDetachedManagedConversation(null)).toBe(false);
  });
});
