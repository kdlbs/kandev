import { describe, expect, it, vi } from "vitest";
import { mrTriggerClass, openDesktopMRReview, openMobileMRReview } from "./mr-topbar-button";
import type { TaskMR } from "@/lib/types/gitlab";

describe("mrTriggerClass", () => {
  it("uses a 44px trigger on mobile", () => {
    expect(mrTriggerClass(true, true)).toContain("h-11");
    expect(mrTriggerClass(true, true)).toContain("w-11");
  });

  it("opens the exact selected MR instead of the first linked MR", () => {
    const setReview = vi.fn();
    openMobileMRReview(setReview, "session-1", {
      host: "https://gitlab.example",
      project_path: "group/b",
      mr_iid: 22,
    } as TaskMR);
    expect(setReview).toHaveBeenCalledWith("session-1", "https://gitlab.example|group/b|22");
  });

  it("confirms desktop MR focus after dockview finishes its layout work", () => {
    const addMRPanel = vi.fn();
    const scheduled: FrameRequestCallback[] = [];
    const schedule = vi.fn((callback: FrameRequestCallback) => {
      scheduled.push(callback);
      return scheduled.length;
    });
    const mr = {
      host: "https://gitlab.example",
      project_path: "group/b",
      mr_iid: 22,
    } as TaskMR;

    openDesktopMRReview(addMRPanel, "session-1", mr, schedule);
    expect(addMRPanel).toHaveBeenCalledTimes(1);
    scheduled.shift()?.(0);
    expect(addMRPanel).toHaveBeenCalledTimes(1);
    scheduled.shift()?.(0);
    expect(addMRPanel).toHaveBeenLastCalledWith("https://gitlab.example|group/b|22", "session-1");
    expect(addMRPanel).toHaveBeenCalledTimes(2);
  });
});
