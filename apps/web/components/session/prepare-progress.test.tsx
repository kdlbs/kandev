import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import type { PrepareStepInfo } from "@/lib/state/slices/session-runtime/types";

let mockSteps: PrepareStepInfo[] = [];
let mockPrepareStatus: "preparing" | "completed" | "failed" = "preparing";
let mockSessionState: string = "STARTING";
let mockMessages: Message[] = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      prepareProgress: {
        bySessionId: {
          "session-1": {
            sessionId: "session-1",
            status: mockPrepareStatus,
            steps: mockSteps,
          },
        },
      },
      taskSessions: {
        items: {
          "session-1": {
            id: "session-1",
            state: mockSessionState,
          },
        },
      },
      sessionAgentctl: {
        itemsBySessionId: {},
      },
      messages: {
        bySession: {
          "session-1": mockMessages,
        },
      },
    }),
}));

import { PrepareProgress } from "./prepare-progress";

describe("PrepareProgress", () => {
  afterEach(() => {
    cleanup();
    mockMessages = [];
  });

  it("hides skipped steps that have no useful details", () => {
    mockPrepareStatus = "preparing";
    mockSessionState = "STARTING";
    mockSteps = [
      {
        name: "Uploading credentials",
        status: "skipped",
      },
      {
        name: "Waiting for agent controller",
        status: "completed",
      },
    ];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.queryByText("Uploading credentials")).toBeNull();
    expect(screen.getByText("Waiting for agent controller")).toBeTruthy();
  });

  it("keeps the fallback notice row visible because it carries a warning", () => {
    mockPrepareStatus = "preparing";
    mockSessionState = "STARTING";
    mockSteps = [
      {
        name: "Reconnecting cloud sandbox",
        status: "skipped",
        warning:
          "Previous sandbox is no longer available — provisioning a fresh one for this branch.",
        warningDetail: "Old sandbox could not be reached.",
        output: "Old sandbox: kandev-old\nNew sandbox: kandev-new\nBranch: feature/foo",
      },
    ];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.getByText("Reconnecting cloud sandbox")).toBeTruthy();
    expect(
      screen.getByText(
        "Previous sandbox is no longer available — provisioning a fresh one for this branch.",
      ),
    ).toBeTruthy();
  });

  it("relabels the header when the only warning is the fallback notice", () => {
    mockPrepareStatus = "completed";
    mockSessionState = "RUNNING";
    mockSteps = [
      {
        name: "Reconnecting cloud sandbox",
        status: "skipped",
        warning:
          "Previous sandbox is no longer available — provisioning a fresh one for this branch.",
      },
      { name: "Creating cloud sandbox", status: "completed" },
      { name: "Waiting for agent controller", status: "completed" },
    ];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.getByText("Environment prepared on a fresh sandbox")).toBeTruthy();
    expect(screen.queryByText("Environment prepared with warnings")).toBeNull();
  });

  it("uses the generic warnings header when warnings are unrelated to fallback", () => {
    mockPrepareStatus = "completed";
    mockSessionState = "RUNNING";
    mockSteps = [
      {
        name: "Uploading credentials",
        status: "completed",
        warning: "Some credentials skipped",
      },
    ];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.getByText("Environment prepared with warnings")).toBeTruthy();
    expect(screen.queryByText("Environment prepared on a fresh sandbox")).toBeNull();
  });
});

function makeSetupScriptMessage(overrides: Partial<Message["metadata"] & object> = {}): Message {
  return {
    id: "msg-1",
    task_id: "task-1" as Message["task_id"],
    session_id: "session-1" as Message["session_id"],
    author_type: "agent",
    content: "Installing deps...\nDone.",
    type: "script_execution",
    created_at: "2026-05-27T19:06:51Z",
    metadata: {
      script_type: "setup",
      command: "make install",
      status: "exited",
      exit_code: 0,
      started_at: "2026-05-27T19:06:51Z",
      completed_at: "2026-05-27T19:06:54Z",
      ...overrides,
    },
  };
}

describe("PrepareProgress per-repo setup script", () => {
  afterEach(() => {
    cleanup();
    mockMessages = [];
  });

  it("renders the per-repo setup script as a step inside the panel", () => {
    // `preparing` keeps the panel auto-expanded so step rows render.
    mockPrepareStatus = "preparing";
    mockSessionState = "STARTING";
    mockSteps = [
      { name: "Validate repository", status: "completed" },
      { name: "Create worktree", status: "completed" },
    ];
    mockMessages = [makeSetupScriptMessage()];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.getByText("Run repository setup script")).toBeTruthy();
    expect(screen.getByText("make install")).toBeTruthy();
  });

  it("marks a failed setup script as a failed step with an error message", () => {
    // `failed` keeps the panel auto-expanded.
    mockPrepareStatus = "failed";
    mockSessionState = "FAILED";
    mockSteps = [{ name: "Create worktree", status: "completed" }];
    mockMessages = [makeSetupScriptMessage({ exit_code: 2 })];

    render(<PrepareProgress sessionId="session-1" />);

    expect(screen.getByText("Script exited with code 2")).toBeTruthy();
  });
});
