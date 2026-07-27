import { describe, expect, it } from "vitest";

import {
  getImproveKandevStepDescription,
  getImproveKandevBrowserStorage,
  getImproveKandevForkBlockedReason,
  initialImproveKandevMode,
  readImproveKandevSkipIntro,
  contributorAccessMessage,
  resolveImproveKandevWorkflow,
  writeImproveKandevSkipIntro,
} from "./improve-kandev-dialog-model";
import type { WorkflowStep } from "@/lib/types/http";

class MemoryStorage {
  private values = new Map<string, string>();

  getItem(key: string) {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string) {
    this.values.set(key, value);
  }

  removeItem(key: string) {
    this.values.delete(key);
  }
}

describe("improve kandev dialog model", () => {
  it("reads and writes the intro preference without requiring browser storage", () => {
    const storage = new MemoryStorage();
    expect(readImproveKandevSkipIntro(undefined)).toBe(false);
    expect(readImproveKandevSkipIntro(storage)).toBe(false);

    writeImproveKandevSkipIntro(storage, true);
    expect(readImproveKandevSkipIntro(storage)).toBe(true);

    writeImproveKandevSkipIntro(storage, false);
    expect(readImproveKandevSkipIntro(storage)).toBe(false);
  });

  it("falls back safely when storage throws", () => {
    const storage = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
      removeItem: () => {
        throw new Error("blocked");
      },
    };
    expect(readImproveKandevSkipIntro(storage)).toBe(false);
    expect(() => writeImproveKandevSkipIntro(storage, true)).not.toThrow();
  });

  it("falls back safely when the browser localStorage property throws", () => {
    const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new Error("blocked");
      },
    });
    try {
      expect(getImproveKandevBrowserStorage()).toBeUndefined();
    } finally {
      if (original) Object.defineProperty(globalThis, "localStorage", original);
      else delete (globalThis as { localStorage?: unknown }).localStorage;
    }
  });

  it("chooses create mode only after the intro has been dismissed", () => {
    expect(initialImproveKandevMode(false)).toBe("intro");
    expect(initialImproveKandevMode(true)).toBe("create");
  });

  it("selects the issue workflow and its start step for Open issue", () => {
    const implementStep = {
      id: "implement-step",
      name: "Improve",
      position: 0,
      is_start_step: true,
    } as WorkflowStep;
    const issueStep = {
      id: "issue-step",
      name: "Open issue",
      position: 0,
      is_start_step: true,
    } as WorkflowStep;
    const ready = {
      data: {
        workflow_id: "implementation",
        issue_workflow_id: "issue-report",
        repository_id: "repo",
        branch: "main",
        fork_status: "unknown" as const,
      },
      steps: [implementStep],
      issueSteps: [issueStep],
    };

    expect(resolveImproveKandevWorkflow(ready, "issue")).toEqual({
      workflowId: "issue-report",
      steps: ready.issueSteps,
      startStep: ready.issueSteps[0],
    });
    expect(resolveImproveKandevWorkflow(ready, "bug")).toEqual({
      workflowId: "implementation",
      steps: ready.steps,
      startStep: ready.steps[0],
    });
  });

  it("blocks only implementation kinds for EMU fork restrictions", () => {
    const message = "Forks are unavailable";
    expect(getImproveKandevForkBlockedReason("bug", "blocked_emu", message)).toBe(message);
    expect(getImproveKandevForkBlockedReason("feature", "blocked_emu", message)).toBe(message);
    expect(getImproveKandevForkBlockedReason("issue", "blocked_emu", message)).toBeNull();
    expect(getImproveKandevForkBlockedReason("bug", "unknown", null)).toBeNull();
  });

  it("describes contributor access for each workflow kind", () => {
    expect(contributorAccessMessage("issue", false)).toContain("does not require");
    expect(contributorAccessMessage("bug", true)).toContain("write access");
    expect(contributorAccessMessage("feature", false)).toContain("fork");
  });

  it("prefers stable workflow step ids for descriptions", () => {
    expect(
      getImproveKandevStepDescription({ id: "improve", name: "Renamed phase" } as WorkflowStep),
    ).toContain("implements the change");
  });
});
