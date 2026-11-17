import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
const fixture = vi.hoisted(() => ({ retry: vi.fn() }));
vi.mock("@/lib/api/domains/builtin-mcp-api", () => ({ retryBuiltinMCPApplication: fixture.retry }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
import {
  BuiltinApplicationFeedback,
  BuiltinSessionEvidenceProvider,
} from "./builtin-session-evidence";
const saved: BuiltinMCPSessionView = {
  task_id: "task",
  session_id: "session",
  workspace_id: "workspace",
  execution_id: "execution",
  binding_state: "unknown",
  walkthrough_availability: "unknown",
  provider_available: false,
  can_retry_application: true,
  application: {
    task_id: "task",
    session_id: "session",
    workspace_id: "workspace",
    request_id: "saved",
    target_revision: 3,
    row_version: 7,
    stage: "failed",
  },
};
function feedback(view: BuiltinMCPSessionView, refresh: () => void) {
  return (
    <BuiltinSessionEvidenceProvider value={{ view, refresh, manage: vi.fn() }}>
      <BuiltinApplicationFeedback />
    </BuiltinSessionEvidenceProvider>
  );
}
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);
it("submits one versioned retry and refreshes the saved status", async () => {
  let done!: () => void;
  fixture.retry.mockReturnValue(
    new Promise<void>((resolve) => {
      done = resolve;
    }),
  );
  const refresh = vi.fn();
  render(feedback(saved, refresh));
  const retry = screen.getByRole("button");
  fireEvent.click(retry);
  expect((retry as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(retry);
  expect(fixture.retry).toHaveBeenCalledExactlyOnceWith("session", "saved", 7);
  await act(async () => done());
  expect(refresh).toHaveBeenCalledTimes(1);
  await waitFor(() => expect((retry as HTMLButtonElement).disabled).toBe(false));
});
it("keeps saved failure visible and restores Retry after a request error", async () => {
  fixture.retry.mockRejectedValue(new Error("connection lost"));
  const refresh = vi.fn();
  render(feedback(saved, refresh));
  fireEvent.click(screen.getByRole("button"));
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toContain("applicationRetryFailed"),
  );
  expect((screen.getByRole("button") as HTMLButtonElement).disabled).toBe(false);
  expect(screen.getByTestId("builtin-mcp-application").getAttribute("data-state")).toBe("failed");
  expect(refresh).toHaveBeenCalledTimes(1);
});
it("ignores a late Retry result after the viewed session changes", async () => {
  let reject!: (error: Error) => void;
  fixture.retry.mockReturnValue(
    new Promise<void>((_, fail) => {
      reject = fail;
    }),
  );
  const previous = vi.fn(),
    current = vi.fn();
  const mounted = render(feedback(saved, previous));
  fireEvent.click(screen.getByRole("button"));
  const other = {
    ...saved,
    session_id: "other",
    application: { ...saved.application!, session_id: "other", request_id: "other-request" },
  };
  mounted.rerender(feedback(other, current));
  await act(async () => reject(new Error("late failure")));
  expect(previous).not.toHaveBeenCalled();
  expect(current).not.toHaveBeenCalled();
  expect(screen.getByRole("status").textContent).not.toContain("applicationRetryFailed");
  expect((screen.getByRole("button") as HTMLButtonElement).disabled).toBe(false);
});
