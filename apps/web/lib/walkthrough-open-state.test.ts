import { describe, expect, it } from "vitest";
import {
  clearOpenWalkthroughTaskId,
  getOpenWalkthroughTaskId,
  setOpenWalkthroughTaskId,
  subscribeOpenWalkthroughTask,
} from "./walkthrough-open-state";

describe("walkthrough open state", () => {
  it("tracks the currently open walkthrough task", () => {
    setOpenWalkthroughTaskId(null);

    setOpenWalkthroughTaskId("task-1");
    expect(getOpenWalkthroughTaskId()).toBe("task-1");

    clearOpenWalkthroughTaskId("other-task");
    expect(getOpenWalkthroughTaskId()).toBe("task-1");

    clearOpenWalkthroughTaskId("task-1");
    expect(getOpenWalkthroughTaskId()).toBeNull();
  });

  it("notifies subscribers when the open task changes", () => {
    setOpenWalkthroughTaskId(null);
    let calls = 0;
    const unsubscribe = subscribeOpenWalkthroughTask(() => {
      calls += 1;
    });

    setOpenWalkthroughTaskId("task-1");
    setOpenWalkthroughTaskId("task-1");
    clearOpenWalkthroughTaskId("task-1");
    unsubscribe();
    setOpenWalkthroughTaskId("task-2");

    expect(calls).toBe(2);
    setOpenWalkthroughTaskId(null);
  });
});
