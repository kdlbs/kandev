import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import { scrollChangesToEnd } from "../tests/git/large-changes-helpers";

const playwright = vi.hoisted(() => {
  const expect = (value: unknown, message?: string) => ({
    async toBeVisible() {
      if (!value) throw new Error("Missing locator");
    },
    toBe(expected: unknown) {
      if (value !== expected) throw new Error(message ?? "Unexpected value");
    },
  });
  return {
    expect: Object.assign(expect, {
      poll: (read: () => unknown | Promise<unknown>) => ({
        async toBeLessThanOrEqual(limit: number) {
          const count = await read();
          if (typeof count !== "number" || count > limit) throw new Error("Unbounded rows");
        },
        async toMatchObject(expected: { within: boolean }) {
          for (let attempt = 0; attempt < 8; attempt += 1) {
            const geometry = (await read()) as { within: boolean };
            if (geometry.within === expected.within) return;
          }
          throw new Error("The measured row remains clipped");
        },
      }),
    }),
  };
});

vi.mock("@playwright/test", () => playwright);

function box(x: number, y: number, width: number, height: number) {
  return { x, y, width, height, top: y, bottom: y + height, left: x, right: x + width };
}

function changesPageFixture(unscrollable = false) {
  const layout = { height: 2_400_066, rowOffset: 2_400_016, top: 0 };
  const owner = {
    clientHeight: 635,
    get scrollHeight() {
      return layout.height;
    },
    get scrollTop() {
      return layout.top;
    },
    set scrollTop(value: number) {
      layout.top = Math.min(value, layout.height - this.clientHeight);
    },
    dispatchEvent: () => true,
    getBoundingClientRect: () => box(18, 122, 731, 635),
  };
  const row = {
    closest: () => owner,
    getBoundingClientRect: () => box(14, 122 + layout.rowOffset - layout.top, 739, 50),
  };
  const ownerLocator = {
    evaluate: async (read: (element: HTMLElement) => unknown) =>
      read(owner as unknown as HTMLElement),
    boundingBox: async () => owner.getBoundingClientRect(),
  };
  const rowLocator = {
    scrollIntoViewIfNeeded: async () => {
      expect(row.getBoundingClientRect().bottom).toBeLessThanOrEqual(
        owner.getBoundingClientRect().bottom,
      );
    },
    evaluate: async (read: (element: HTMLElement) => unknown) =>
      read(row as unknown as HTMLElement),
    boundingBox: async () => row.getBoundingClientRect(),
  };
  const page = {
    getByTestId: (id: string) => (id === "changes-panel-scroll-owner" ? ownerLocator : rowLocator),
    locator: () => ({
      count: async () => {
        // A resize measurement lands after the helper's initial positioning.
        // The observed failure left 26px of additional scroll range unused.
        layout.height = 2_400_092;
        layout.rowOffset = unscrollable ? 2_400_100 : 2_400_030;
        return 20;
      },
    }),
  } as unknown as Page;
  return { page, owner, row };
}

beforeEach(() => {
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    callback(0);
    return 0;
  });
  vi.stubGlobal("window", { innerWidth: 767 });
});
afterEach(() => vi.unstubAllGlobals());

describe("scrollChangesToEnd after virtualized resize measurement", () => {
  it("repositions the row when its final measurement adds scroll range", async () => {
    const { page, owner, row } = changesPageFixture();
    await expect(scrollChangesToEnd(page, "last-file")).resolves.toBeUndefined();
    expect(owner.scrollTop).toBe(owner.scrollHeight - owner.clientHeight);
    expect(row.getBoundingClientRect().bottom).toBeLessThanOrEqual(
      owner.getBoundingClientRect().bottom,
    );
  });

  it("still rejects a row that cannot fit inside the scroll owner", async () => {
    const { page } = changesPageFixture(true);
    await expect(scrollChangesToEnd(page, "last-file")).rejects.toThrow();
  });
});
