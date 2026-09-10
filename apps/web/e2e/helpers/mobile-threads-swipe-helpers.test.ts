import type { Page } from "@playwright/test";
import { describe, expect, it } from "vitest";
import { swipeDeckLeft } from "../tests/task/mobile-threads-swipe-helpers";

describe("swipeDeckLeft cleanup", () => {
  it.each(["touchStart", "touchEnd"])("detaches CDP when %s fails", async (failure) => {
    const events: string[] = [];
    const page = {
      getByTestId: () => ({ boundingBox: async () => ({ x: 0, y: 56, width: 360, height: 700 }) }),
      context: () => ({
        newCDPSession: async () => ({
          send: async (_method: string, { type }: { type: string }) => {
            events.push(type);
            if (type === failure) throw new Error(failure);
          },
          detach: async () => {
            events.push("detach");
          },
        }),
      }),
    } as unknown as Page;

    await expect(swipeDeckLeft(page)).rejects.toThrow(failure);
    expect(events.slice(-2)).toEqual(["touchEnd", "detach"]);
  });
});
