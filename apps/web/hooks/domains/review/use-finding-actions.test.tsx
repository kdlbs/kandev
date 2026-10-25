import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, within } from "@testing-library/react";
import { registerReviewHandlers } from "@/lib/ws/handlers/review";
import {
  acknowledge,
  click,
  finding,
  mountFindings,
  pendingRequest,
  refuse,
  status,
} from "./use-finding-actions.test-utils";

const NAV_ITEM = "review-finding-nav-item";
const UNDO_DENIED = "Undo denied";

const boundary = vi.hoisted(() => ({ request: vi.fn(), report: vi.fn() }));
vi.mock("@/lib/ws/connection", async (original) => ({
  ...(await original<Record<string, unknown>>()),
  getWebSocketClient: () => ({ request: boundary.request }),
}));
vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: boundary.report,
}));

beforeEach(() => {
  boundary.request.mockReset();
  boundary.report.mockReset();
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.9
it("retains acknowledged Dismiss after older Resolve rejects", async () => {
  const resolve = pendingRequest();
  const undo = pendingRequest();
  const dismiss = pendingRequest();
  boundary.request
    .mockReturnValueOnce(resolve.promise)
    .mockReturnValueOnce(undo.promise)
    .mockReturnValueOnce(dismiss.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  expect(status(container)).toBe("resolved");
  click(container, "reopen");
  await acknowledge(undo, finding({ status: "open" }));
  click(container, "dismiss");
  const accepted = finding({
    status: "dismissed",
    updated_at: "2026-10-08T07:10:00Z",
    body: "Dismissed after inspection.",
  });
  await acknowledge(dismiss, accepted);
  expect(status(container)).toBe("dismissed");
  expect(within(container).getByText("No open findings.")).toBeTruthy();
  expect(boundary.request.mock.calls).toEqual([
    ["task.review.finding.update", { finding_id: "f-a", status: "resolved" }],
    ["task.review.finding.update", { finding_id: "f-a", status: "open" }],
    ["task.review.finding.update", { finding_id: "f-a", status: "dismissed" }],
  ]);
  await refuse(resolve, new Error("Resolve was rejected"));
  expect(status(container)).toBe("dismissed");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
  expect(within(container).queryByTestId(NAV_ITEM)).toBeNull();
  expect(boundary.report).toHaveBeenCalledOnce();
  expect(within(container).getByText("Resolve was rejected")).toBeTruthy();
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.12
it("rolls a current rejected Resolve back and displays its error", async () => {
  const request = pendingRequest();
  boundary.request.mockReturnValueOnce(request.promise);
  const original = finding();
  const { container, store } = mountFindings([original]);
  click(container, "resolve");
  expect(status(container)).toBe("resolved");
  await refuse(request, new Error("Request denied"));
  expect(status(container)).toBe("open");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([original]);
  expect(within(container).getByTestId(NAV_ITEM)).toBeTruthy();
  expect(within(container).getByText("Request denied")).toBeTruthy();
  expect(boundary.report).toHaveBeenCalledOnce();
});

it("publishes the complete current Resolve acknowledgement", async () => {
  const request = pendingRequest();
  boundary.request.mockReturnValueOnce(request.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  const accepted = finding({
    status: "resolved",
    resolved_at: "2026-10-08T07:05:00Z",
    updated_at: "2026-10-08T07:05:00Z",
  });
  await acknowledge(request, accepted);
  expect(status(container)).toBe("resolved");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
  expect(within(container).getByText("No open findings.")).toBeTruthy();
  expect(boundary.report).not.toHaveBeenCalled();
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.9, AC-AGENTS-NATIVE-CODE-REVIEW-001.11
it("shares ownership across separate consumers when older Resolve succeeds", async () => {
  const resolve = pendingRequest(),
    undo = pendingRequest(),
    dismiss = pendingRequest();
  boundary.request
    .mockReturnValueOnce(resolve.promise)
    .mockReturnValueOnce(undo.promise)
    .mockReturnValueOnce(dismiss.promise);
  const { container, store } = mountFindings([finding()], 2);
  click(container, "resolve", 0);
  click(container, "reopen", 1);
  await acknowledge(undo, finding());
  click(container, "dismiss", 1);
  const accepted = finding({ status: "dismissed", updated_at: "2026-10-08T08:00:00Z" });
  await acknowledge(dismiss, accepted);
  await acknowledge(resolve, finding({ status: "resolved", resolved_at: "2026-10-08T07:15:00Z" }));
  expect(status(container, 0)).toBe("dismissed");
  expect(status(container, 1)).toBe("dismissed");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
  expect(within(container).getAllByText("No open findings.")).toHaveLength(2);
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.10
it("keeps newer optimism and rolls failed Undo back to acknowledged Resolve", async () => {
  const resolve = pendingRequest(),
    undo = pendingRequest();
  boundary.request.mockReturnValueOnce(resolve.promise).mockReturnValueOnce(undo.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  click(container, "reopen");
  const accepted = finding({
    status: "resolved",
    body: "Confirmed Resolve",
    resolved_at: "2026-10-08T07:30:00Z",
  });
  await acknowledge(resolve, accepted);
  expect(status(container)).toBe("open");
  expect(within(container).getByTestId(NAV_ITEM)).toBeTruthy();
  await refuse(undo, new Error(UNDO_DENIED));
  expect(status(container)).toBe("resolved");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
  expect(within(container).getByText(UNDO_DENIED)).toBeTruthy();
});

it("publishes older acknowledgement after newest failure", async () => {
  const resolve = pendingRequest(),
    undo = pendingRequest();
  boundary.request.mockReturnValueOnce(resolve.promise).mockReturnValueOnce(undo.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  click(container, "reopen");
  await refuse(undo, new Error(UNDO_DENIED));
  expect(status(container)).toBe("open");
  const accepted = finding({ status: "resolved", updated_at: "2026-10-08T07:45:00Z" });
  await acknowledge(resolve, accepted);
  expect(status(container)).toBe("resolved");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
});

it("retains pre-overlap row when both actions fail", async () => {
  const resolve = pendingRequest(),
    undo = pendingRequest();
  boundary.request.mockReturnValueOnce(resolve.promise).mockReturnValueOnce(undo.promise);
  const original = finding();
  const { container, store } = mountFindings([original]);
  click(container, "resolve");
  click(container, "reopen");
  await refuse(undo, new Error(UNDO_DENIED));
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([original]);
  await refuse(resolve, new Error("Resolve denied"));
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([original]);
  expect(status(container)).toBe("open");
  expect(boundary.report).toHaveBeenCalledTimes(2);
});

it("rolls rejected Dismiss back to acknowledged Undo rather than its optimistic input", async () => {
  const resolve = pendingRequest(),
    undo = pendingRequest(),
    dismiss = pendingRequest();
  boundary.request
    .mockReturnValueOnce(resolve.promise)
    .mockReturnValueOnce(undo.promise)
    .mockReturnValueOnce(dismiss.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  await acknowledge(resolve, finding({ status: "resolved" }));
  click(container, "reopen");
  click(container, "dismiss");
  const accepted = finding({
    status: "open",
    body: "Acknowledged Undo",
    resolved_at: null,
    updated_at: "2026-10-08T08:05:00Z",
  });
  await acknowledge(undo, accepted);
  expect(status(container)).toBe("dismissed");
  await refuse(dismiss, new Error("Dismiss denied"));
  expect(status(container)).toBe("open");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
});

it("does not confuse repeated resolved statuses with the same action", async () => {
  const older = pendingRequest(),
    undo = pendingRequest(),
    newer = pendingRequest();
  boundary.request
    .mockReturnValueOnce(older.promise)
    .mockReturnValueOnce(undo.promise)
    .mockReturnValueOnce(newer.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  click(container, "reopen");
  await acknowledge(undo, finding());
  click(container, "resolve");
  const accepted = finding({ status: "resolved", resolved_at: "2026-10-08T08:15:00Z" });
  await acknowledge(newer, accepted);
  await refuse(older, new Error("Older Resolve denied"));
  expect(status(container)).toBe("resolved");
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.12
it.each([
  { action: "dismiss", before: "open", after: "dismissed" },
  { action: "reopen", before: "dismissed", after: "open" },
] as const)(
  "retains current $action success and failure behavior",
  async ({ action, before, after }) => {
    const acceptedRequest = pendingRequest(),
      deniedRequest = pendingRequest();
    boundary.request
      .mockReturnValueOnce(acceptedRequest.promise)
      .mockReturnValueOnce(deniedRequest.promise);
    const original = finding({ status: before });
    const { container, store } = mountFindings([original]);
    click(container, action);
    expect(status(container)).toBe(after);
    const accepted = finding({
      status: after,
      body: "Returned row metadata",
      updated_at: "2026-10-08T08:20:00Z",
    });
    await acknowledge(acceptedRequest, accepted);
    expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
    const reverse = after === "open" ? "dismiss" : "reopen";
    click(container, reverse);
    await refuse(deniedRequest, new Error("Action denied"));
    expect(status(container)).toBe(after);
    expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([accepted]);
    expect(within(container).getByText("Could not update finding")).toBeTruthy();
    expect(boundary.report).toHaveBeenCalledOnce();
  },
);

it("keeps non-Error rejection fallback feedback", async () => {
  const request = pendingRequest();
  boundary.request.mockReturnValueOnce(request.promise);
  const { container } = mountFindings();
  click(container, "resolve");
  await refuse(request, null);
  expect(status(container)).toBe("open");
  expect(within(container).getByText("An error occurred")).toBeTruthy();
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.11
it("keeps different findings independent across interleaved completions", async () => {
  const first = pendingRequest(),
    second = pendingRequest();
  boundary.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const { container, store } = mountFindings([finding(), finding({ id: "f-b" })]);
  click(container, "resolve");
  click(container, "dismiss", 0, "f-b");
  await acknowledge(second, finding({ id: "f-b", status: "dismissed" }));
  await refuse(first, new Error("First denied"));
  expect(status(container)).toBe("open");
  expect(status(container, 0, "f-b")).toBe("dismissed");
  expect(store.getState().taskReview.findingsByTaskId["task-a"].map((row) => row.status)).toEqual([
    "open",
    "dismissed",
  ]);
});

it("keeps equal finding IDs in different tasks independent", async () => {
  const first = pendingRequest(),
    second = pendingRequest();
  boundary.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const { container, store } = mountFindings([finding(), finding({ task_id: "task-b" })]);
  click(container, "resolve");
  act(() => store.getState().setActiveTask("task-b"));
  click(container, "dismiss");
  await acknowledge(second, finding({ task_id: "task-b", status: "dismissed" }));
  await refuse(first, new Error("Task A denied"));
  expect(status(container)).toBe("dismissed");
  expect(store.getState().taskReview.findingsByTaskId["task-a"][0].status).toBe("open");
  expect(store.getState().taskReview.findingsByTaskId["task-b"][0].status).toBe("dismissed");
});

it("keeps separate real provider stores independent", async () => {
  const first = pendingRequest(),
    second = pendingRequest();
  boundary.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const a = mountFindings(),
    b = mountFindings();
  click(a.container, "resolve");
  click(b.container, "dismiss");
  await acknowledge(second, finding({ status: "dismissed" }));
  await acknowledge(first, finding({ status: "resolved" }));
  expect(status(a.container)).toBe("resolved");
  expect(status(b.container)).toBe("dismissed");
  expect(a.store).not.toBe(b.store);
});

// @covers AC-AGENTS-NATIVE-CODE-REVIEW-001.13
it.each(["snapshot", "clear", "supersession"] as const)(
  "preserves %s writer authority through late success and failure",
  async (writer) => {
    const first = pendingRequest(),
      second = pendingRequest();
    boundary.request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { container, store } = mountFindings();
    click(container, "resolve");
    click(container, "reopen");
    const replacement = finding({
      id: writer === "supersession" ? "replacement" : "f-a",
      status: "dismissed",
      body: "Independent replacement",
    });
    act(() => {
      if (writer === "snapshot")
        store.getState().setTaskReview("task-a", { runs: [], findings: [replacement] });
      if (writer === "clear") store.getState().clearTaskReviewState("task-a");
      if (writer === "supersession")
        store.getState().addReviewFindings("task-a", [replacement], ["f-a"]);
    });
    const expected = writer === "clear" ? undefined : [replacement];
    await acknowledge(first, finding({ status: "resolved" }));
    expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual(expected);
    await refuse(second, new Error("Old Undo denied"));
    expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual(expected);
    expect(within(container).queryByTestId(NAV_ITEM)).toBeNull();
  },
);

it("starts replacement actions from their own baseline and preserves unseen WS updates", async () => {
  const old = pendingRequest(),
    current = pendingRequest();
  boundary.request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
  const { container, store } = mountFindings();
  click(container, "resolve");
  const replacement = finding({ status: "dismissed", body: "Live authority" });
  act(() => store.getState().setTaskReview("task-a", { runs: [], findings: [replacement] }));
  click(container, "reopen");
  await acknowledge(old, finding({ status: "resolved" }));
  expect(status(container)).toBe("open");
  const unseen = finding({ id: "unseen", status: "resolved" });
  act(() =>
    registerReviewHandlers(store)["task.review.finding_updated"]!({
      action: "task.review.finding_updated",
      payload: { task_id: "task-a", finding: unseen },
    } as never),
  );
  await refuse(current, new Error("Current Undo denied"));
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([replacement, unseen]);
  expect(status(container)).toBe("dismissed");
});

it("cedes ownership to a synchronous subscriber replacement instead of adopting setter readback", async () => {
  const request = pendingRequest();
  boundary.request.mockReturnValueOnce(request.promise);
  const { container, store } = mountFindings();
  const replacement = finding({ status: "dismissed", body: "Subscriber authority" });
  let replaced = false;
  const unsubscribe = store.subscribe((state) => {
    if (replaced || state.taskReview.findingsByTaskId["task-a"][0].status !== "resolved") return;
    replaced = true;
    state.updateReviewFinding("task-a", replacement);
  });
  click(container, "resolve");
  expect(replaced).toBe(true);
  expect(status(container)).toBe("dismissed");
  await acknowledge(request, finding({ status: "resolved" }));
  expect(store.getState().taskReview.findingsByTaskId["task-a"]).toEqual([replacement]);
  expect(status(container)).toBe("dismissed");
  unsubscribe();
});
