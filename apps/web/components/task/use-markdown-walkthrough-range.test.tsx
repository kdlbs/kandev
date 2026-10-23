import { renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
const state = vi.hoisted(() => ({
  tasks: { activeTaskId: "task", activeSessionId: "session" },
  walkthroughs: {
    activeStepByTaskId: { task: 0 },
    byTaskId: {
      task: {
        steps: [
          { file: "guide.md", repo: "other", line: 3, line_end: 5, text: "One" },
          { file: "guide.md", repo: "other", line: 3, line_end: 9, text: "Two" },
        ],
      },
    },
  },
  open: true,
}));
vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));
vi.mock("@/lib/walkthrough-open-state", () => ({ useIsWalkthroughOpenForTask: () => state.open }));
import { useMarkdownWalkthroughRange } from "./use-markdown-walkthrough-range";
afterEach(() => {
  state.open = true;
  state.walkthroughs.activeStepByTaskId.task = 0;
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.16
it("resolves same-file steps by both endpoints and the exact file/session repository scope", () => {
  const props = { path: "docs/guide.md", repo: "other", taskId: "task", sessionId: "session" };
  const { result, rerender } = renderHook((input) => useMarkdownWalkthroughRange(input), {
    initialProps: props,
  });
  expect(result.current).toMatchObject({ startLine: 3, endLine: 5, repositoryName: "other" });
  const firstKey = result.current!.key;
  state.walkthroughs.activeStepByTaskId.task = 1;
  rerender(props);
  expect(result.current).toMatchObject({ startLine: 3, endLine: 9 });
  expect(result.current!.key).not.toBe(firstKey);
  rerender({ ...props, repo: "primary" });
  expect(result.current).toBeNull();
  rerender({ ...props, sessionId: "other-session" });
  expect(result.current).toBeNull();
  rerender(props);
  expect(result.current).not.toBeNull();
  state.open = false;
  rerender(props);
  expect(result.current).toBeNull();
});
