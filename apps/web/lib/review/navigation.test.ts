import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskReviewFinding } from "@/lib/types/review";
import {
  FINDING_DOM_ATTR,
  FINDING_FLASH_CLASS,
  NAVIGATE_FINDING_EVENT,
  findingSelector,
  navigateToFinding,
  scrollToFinding,
} from "./navigation";

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f1",
    run_id: "r1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "apps/web/a.ts",
    start_line: 12,
    end_line: 12,
    side: "additions",
    severity: "blocker",
    category: "correctness",
    title: "Nil dereference",
    body: "x can be nil",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

function mountFindingCard(id: string): HTMLElement {
  const el = document.createElement("div");
  el.setAttribute(FINDING_DOM_ATTR, id);
  el.scrollIntoView = vi.fn();
  document.body.appendChild(el);
  return el;
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

describe("findingSelector", () => {
  it("escapes ids for use in a CSS selector", () => {
    expect(findingSelector("a:b")).toBe(`[${FINDING_DOM_ATTR}="a\\:b"]`);
  });
});

describe("scrollToFinding", () => {
  it("scrolls to and flashes a present card, resolving true", async () => {
    const el = mountFindingCard("f1");
    const found = await scrollToFinding("f1", (cb) => cb());
    expect(found).toBe(true);
    expect(el.scrollIntoView).toHaveBeenCalled();
    expect(el.classList.contains(FINDING_FLASH_CLASS)).toBe(true);
  });

  it("finds a card that renders after a few frames", async () => {
    let frames = 0;
    const found = await scrollToFinding("late", (cb) => {
      frames += 1;
      if (frames === 3) mountFindingCard("late");
      cb();
    });
    expect(found).toBe(true);
  });

  it("resolves false when the card never appears", async () => {
    const found = await scrollToFinding("missing", (cb) => cb());
    expect(found).toBe(false);
  });
});

describe("navigateToFinding", () => {
  it("selects the finding's file and dispatches the navigate event", async () => {
    const target = finding({ id: "f9", repository_name: "kandev", file_path: "a.ts" });
    mountFindingCard("f9");
    const selectFile = vi.fn();
    const events: string[] = [];
    const listener = (e: Event) => {
      events.push((e as CustomEvent<{ findingId: string }>).detail.findingId);
    };
    window.addEventListener(NAVIGATE_FINDING_EVENT, listener);

    await navigateToFinding(target, selectFile);

    // Composite <repo>\x00<path> key so cross-repo same-named files stay distinct.
    expect(selectFile).toHaveBeenCalledWith("kandev\x00a.ts");
    expect(events).toEqual(["f9"]);
    window.removeEventListener(NAVIGATE_FINDING_EVENT, listener);
  });
});
