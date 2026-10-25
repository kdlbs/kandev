import type { Page } from "@playwright/test";
import { describe, expect, it } from "vitest";
import { swipeDeckLeft, swipeDeckRight } from "../tests/task/mobile-threads-swipe-helpers";

describe("swipeDeckLeft cleanup", () => {
  it("swipes right toward an earlier thread when the selected thread is last", async () => {
    const positions: number[] = [];
    const page = {
      getByTestId: () => ({ boundingBox: async () => ({ x: 0, y: 56, width: 360, height: 700 }) }),
      context: () => ({
        newCDPSession: async () => ({
          send: async (_method: string, { touchPoints }: { touchPoints: { x: number }[] }) => {
            if (touchPoints.length) positions.push(touchPoints[0].x);
          },
          detach: async () => {},
        }),
      }),
    } as unknown as Page;

    await swipeDeckRight(page);
    expect(positions[0]).toBeCloseTo(54);
    expect(positions.at(-1)).toBeCloseTo(306);
  });

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
