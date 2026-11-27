import { describe, expect, it } from "vitest";
import {
  isDetachedManagedConversation,
  managedConversationBlocksQueueDelivery,
} from "./retained-managed-conversation";

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

describe("managedConversationBlocksQueueDelivery", () => {
  it("holds delivery while paused or detached and allows it after resume", () => {
    const retained = { "kandev.managed_retained": true };
    expect(managedConversationBlocksQueueDelivery({ metadata: retained })).toBe(true);
    expect(
      managedConversationBlocksQueueDelivery({
        metadata: { ...retained, "kandev.desired_paused": true },
      }),
    ).toBe(true);
    expect(
      managedConversationBlocksQueueDelivery({
        metadata: { ...retained, "kandev.desired_paused": false, "kandev.detached": true },
      }),
    ).toBe(true);
    expect(
      managedConversationBlocksQueueDelivery({
        metadata: { ...retained, "kandev.desired_paused": false },
      }),
    ).toBe(false);
    expect(
      managedConversationBlocksQueueDelivery({ metadata: { "kandev.desired_paused": true } }),
    ).toBe(false);
    expect(managedConversationBlocksQueueDelivery(null)).toBe(false);
  });
});
