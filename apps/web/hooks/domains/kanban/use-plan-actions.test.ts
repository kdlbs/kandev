import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ContextFile } from "@/lib/state/context-files-store";

const mockGetState = vi.fn();

vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: { getState: () => mockGetState() },
}));

import { buildImplementPlanContent, readContextFilesMeta } from "./use-plan-actions";

describe("buildImplementPlanContent", () => {
  it("appends the kandev-system block to user text", () => {
    const out = buildImplementPlanContent("ship it");
    expect(out.startsWith("ship it\n\n")).toBe(true);
    expect(out).toContain("<kandev-system>");
    expect(out).toContain("get_task_plan_kandev");
    expect(out).toContain("</kandev-system>");
  });

  it("trims whitespace from user text", () => {
    const out = buildImplementPlanContent("   hello  \n");
    expect(out.startsWith("hello\n\n")).toBe(true);
  });

  it("uses default visible text when input is empty", () => {
    expect(buildImplementPlanContent("")).toMatch(/^Implement the plan\n\n/);
    expect(buildImplementPlanContent("   ")).toMatch(/^Implement the plan\n\n/);
  });
});

function file(path: string, name: string, pinned?: boolean): ContextFile {
  return { path, name, pinned };
}

describe("readContextFilesMeta", () => {
  const sessionId = "sess-1";
  const appFilePath = "src/app.ts";
  const appFileName = "app.ts";

  beforeEach(() => vi.clearAllMocks());

  it("returns an empty array when the session has no context files", () => {
    mockGetState.mockReturnValue({ filesBySessionId: {} });
    expect(readContextFilesMeta(sessionId)).toEqual([]);
  });

  it("maps real files to {path, name} pairs", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file(appFilePath, appFileName), file("README.md", "README.md")],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([
      { path: appFilePath, name: appFileName },
      { path: "README.md", name: "README.md" },
    ]);
  });

  it("filters out the special plan:context path", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file("plan:context", "Plan"), file(appFilePath, appFileName)],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([{ path: appFilePath, name: appFileName }]);
  });

  it("filters out prompt: prefixed paths", () => {
    mockGetState.mockReturnValue({
      filesBySessionId: {
        [sessionId]: [file("prompt:my-prompt", "My Prompt"), file(appFilePath, appFileName)],
      },
    });
    expect(readContextFilesMeta(sessionId)).toEqual([{ path: appFilePath, name: appFileName }]);
  });
});
