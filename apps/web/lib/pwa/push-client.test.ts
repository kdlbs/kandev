import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const mocks = vi.hoisted(() => ({ deletePushSubscription: vi.fn() }));

vi.mock("@/lib/api/domains/push-api", () => ({
  deletePushSubscription: mocks.deletePushSubscription,
}));

import { unsubscribeCurrentPushSubscription } from "./push-client";

const ENDPOINT = "https://push.example/device";

function setCurrentSubscription(
  subscription: { endpoint: string; unsubscribe: () => Promise<boolean> } | null,
) {
  vi.stubGlobal("navigator", {
    serviceWorker: {
      getRegistration: vi
        .fn()
        .mockResolvedValue(
          subscription
            ? { pushManager: { getSubscription: vi.fn().mockResolvedValue(subscription) } }
            : undefined,
        ),
    },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.deletePushSubscription.mockResolvedValue({ success: true });
});

afterEach(() => vi.unstubAllGlobals());

describe("unsubscribeCurrentPushSubscription", () => {
  it("removes the current browser subscription from the server and browser", async () => {
    const unsubscribe = vi.fn().mockResolvedValue(true);
    setCurrentSubscription({ endpoint: ENDPOINT, unsubscribe });

    await unsubscribeCurrentPushSubscription();

    expect(mocks.deletePushSubscription).toHaveBeenCalledWith(
      ENDPOINT,
      expect.objectContaining({
        init: expect.objectContaining({ signal: expect.any(AbortSignal) }),
      }),
    );
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it("does nothing when no service worker subscription exists", async () => {
    setCurrentSubscription(null);

    await expect(unsubscribeCurrentPushSubscription()).resolves.toBeUndefined();
    expect(mocks.deletePushSubscription).not.toHaveBeenCalled();
  });

  it("treats an already-removed server subscription as complete", async () => {
    const unsubscribe = vi.fn().mockResolvedValue(true);
    setCurrentSubscription({ endpoint: ENDPOINT, unsubscribe });
    mocks.deletePushSubscription.mockRejectedValue(new ApiError("missing", 404, null));

    await expect(unsubscribeCurrentPushSubscription()).resolves.toBeUndefined();
    expect(unsubscribe).toHaveBeenCalledOnce();
  });
});
