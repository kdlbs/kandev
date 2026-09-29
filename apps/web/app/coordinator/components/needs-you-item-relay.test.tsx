/* eslint-disable sonarjs/no-duplicate-string, max-lines-per-function -- one file drives the whole card through a shared harness */
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ClarificationOutcome } from "@/hooks/domains/session/use-clarification-group";
import type { Relay, RelayRead } from "@/lib/api/domains/coordinator-relay-api";
import type { AttentionTask, NeedsYouQuestionItem } from "@/lib/coordinator/attention";
import type { Message } from "@/lib/types/http";
import { NeedsYouItemsPanel } from "./needs-you-items-panel";

const mocks = vi.hoisted(() => ({
  readRelay: vi.fn(),
  request: vi.fn(),
  toast: Object.assign(vi.fn(), { warning: vi.fn(), error: vi.fn() }),
  phase3: { on: true },
  panelProps: { current: null as null | Record<string, unknown> },
}));

vi.mock("@/lib/api/domains/coordinator-relay-api", () => ({ readRelay: mocks.readRelay }));
vi.mock("@/lib/toast/sonner", () => ({ toast: mocks.toast }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
  useWebSocketClient: () => null,
}));
vi.mock("@/hooks/domains/settings/use-coordinator-phase3-effective", () => ({
  useCoordinatorPhase3Effective: () => mocks.phase3.on,
}));
vi.mock("@/components/task/chat/clarification-panel-section", () => ({
  ClarificationPanelSection: (props: Record<string, unknown>) => {
    mocks.panelProps.current = props;
    return <div data-testid="clarification-panel-stub" />;
  },
}));
vi.mock("../use-needs-you-focus", () => ({ useNeedsYouFocusAfterDecision: () => {} }));
vi.mock("../use-needs-you-navigation", () => ({
  useNeedsYouFormNavigation: () => ({
    autoOpenProposalId: null,
    autoOpenForm: null,
    onAutoFormOpened: () => {},
  }),
}));

afterEach(cleanup);
beforeEach(() => {
  mocks.readRelay.mockReset();
  mocks.request.mockReset();
  mocks.toast.mockReset();
  mocks.toast.warning.mockReset();
  mocks.toast.error.mockReset();
  mocks.phase3.on = true;
  mocks.panelProps.current = null;
});

const NONE: Relay = { task_id: "t-1", session_id: "s-1", clarification: null, permission: null };

function permissionRelay(requestId = "r-1", options?: unknown[]): Relay {
  const message = {
    id: "m-1",
    task_id: "t-1",
    session_id: "s-1",
    content: "Run ls",
    metadata: {
      request_id: requestId,
      pending_id: "p-1",
      action_details: { command: "ls -la" },
      options: options ?? [
        { option_id: "allow", name: "Allow", kind: "allow_once" },
        { option_id: "deny", name: "Deny", kind: "reject_once" },
      ],
    },
  } as unknown as Message;
  return { ...NONE, permission: { message } };
}

function clarificationRelay(): Relay {
  const messages = [
    { id: "q-1", metadata: { pending_id: "b-1", question: { id: "q", prompt: "Which one?" } } },
  ] as unknown as Message[];
  return { ...NONE, clarification: { pending_id: "b-1", context: "", messages } };
}

const ok = (relay: Relay): RelayRead => ({ kind: "ok", relay });

function question(
  id: string,
  pendingAction: "clarification" | "permission" = "permission",
): NeedsYouQuestionItem {
  const task: AttentionTask = {
    id,
    title: `Task ${id}`,
    identifier: id.toUpperCase(),
    statusSummary: { pending_action: pendingAction },
  };
  return { kind: "question", id, task, pendingAction, referenceTimeMs: 0, ageMs: 60_000 };
}

function panel(items: NeedsYouQuestionItem[], canManage = true): ReactElement {
  return (
    <TooltipProvider>
      <NeedsYouItemsPanel
        items={items}
        workingCount={0}
        inputsLoaded
        workspaceId="ws-1"
        coordinatorId="co-1"
        coordinatorName="Ops"
        canManage={canManage}
        attentionMaps={{
          stepNameByTaskId: new Map(),
          workflowNameById: new Map(),
          stepNameByWorkflowStep: new Map(),
          openTasksById: new Map(),
        }}
        computeNeedsYouCount={() => 0}
      />
    </TooltipProvider>
  );
}

async function renderPermissionItem(read: RelayRead = ok(permissionRelay())) {
  mocks.readRelay.mockResolvedValue(read);
  const view = render(panel([question("t-1")]));
  const button = await screen.findByTestId("needs-you-answer-here");
  return { ...view, button };
}

describe("Answer here availability", () => {
  it("is offered to a manager once the read carries the item's kind, replacing the phase 1 text", async () => {
    await renderPermissionItem();
    expect(screen.queryByText("Your answer, on the task")).toBeNull();
    expect(screen.getByText("Open task")).toBeTruthy();
  });

  it("keeps the phase 1 text when the read has no answerable item", async () => {
    mocks.readRelay.mockResolvedValue(ok(NONE));
    render(panel([question("t-1")]));
    await waitFor(() => expect(mocks.readRelay).toHaveBeenCalled());
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
    expect(screen.getByText("Your answer, on the task")).toBeTruthy();
  });

  it("keeps the phase 1 text when the read fails", async () => {
    mocks.readRelay.mockResolvedValue({ kind: "failed" });
    render(panel([question("t-1")]));
    await waitFor(() => expect(mocks.readRelay).toHaveBeenCalled());
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
  });

  it("makes no relay read for a reader", () => {
    render(panel([question("t-1")], false));
    expect(mocks.readRelay).not.toHaveBeenCalled();
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
    expect(screen.getByText("Your answer, on the task")).toBeTruthy();
  });

  it("makes no relay read while phase 3 is not effective", () => {
    mocks.phase3.on = false;
    render(panel([question("t-1")]));
    expect(mocks.readRelay).not.toHaveBeenCalled();
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
  });
});

describe("permission card", () => {
  it("shows the title, the action summary and the chat's buttons on expand", async () => {
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    expect(await screen.findByText("Run ls")).toBeTruthy();
    expect(screen.getByTestId("permission-action-detail").textContent).toBe("ls -la");
    expect(screen.getByTestId("permission-reject")).toBeTruthy();
    expect(screen.getByTestId("permission-approve")).toBeTruthy();
    expect(screen.queryByTestId("permission-allow-always")).toBeNull();
  });

  it("falls back to the permission-required title when the message content is empty", async () => {
    const relay = permissionRelay();
    (relay.permission!.message as { content: string }).content = "";
    const { button } = await renderPermissionItem(ok(relay));
    fireEvent.click(button);
    expect(await screen.findByText("Permission Required")).toBeTruthy();
  });

  it("sends the chat's request for Approve and collapses on success, then rereads once", async () => {
    mocks.request.mockResolvedValue({});
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    const readsBeforeAnswer = mocks.readRelay.mock.calls.length;
    fireEvent.click(await screen.findByTestId("permission-approve"));
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith("permission.respond", {
        task_id: "t-1",
        session_id: "s-1",
        request_id: "r-1",
        pending_id: "p-1",
        option_id: "allow",
        cancelled: false,
        rejected: false,
      }),
    );
    await waitFor(() => expect(screen.queryByTestId("permission-answer")).toBeNull());
    await waitFor(() =>
      expect(mocks.readRelay.mock.calls.length).toBeGreaterThan(readsBeforeAnswer),
    );
  });

  it("sends rejected for Deny", async () => {
    mocks.request.mockResolvedValue({});
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    fireEvent.click(await screen.findByTestId("permission-reject"));
    await waitFor(() =>
      expect(mocks.request).toHaveBeenCalledWith(
        "permission.respond",
        expect.objectContaining({ option_id: "deny", rejected: true, cancelled: false }),
      ),
    );
  });

  it("shows Approve disabled and sends nothing when there is no allow option", async () => {
    const relay = permissionRelay("r-1", [
      { option_id: "deny", name: "Deny", kind: "reject_once" },
    ]);
    const { button } = await renderPermissionItem(ok(relay));
    fireEvent.click(button);
    const approve = await screen.findByTestId("permission-approve");
    expect((approve as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(approve);
    expect(mocks.request).not.toHaveBeenCalled();
  });

  it("disables every button while in flight, so one choice sends one request", async () => {
    let finish: (value: unknown) => void = () => {};
    mocks.request.mockImplementation(() => new Promise((resolve) => (finish = resolve)));
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    const approve = await screen.findByTestId("permission-approve");
    fireEvent.click(approve);
    await waitFor(() => expect((approve as HTMLButtonElement).disabled).toBe(true));
    expect((screen.getByTestId("permission-reject") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(approve);
    expect(mocks.request).toHaveBeenCalledTimes(1);
    await act(async () => finish({}));
  });

  it("shows an inline error with Try again that resends the same option, buttons re-enabled", async () => {
    mocks.request.mockRejectedValueOnce(new Error("timeout")).mockResolvedValueOnce({});
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    fireEvent.click(await screen.findByTestId("permission-reject"));
    const error = await screen.findByTestId("permission-answer-error");
    expect(error.textContent).toContain("Could not respond to the permission request.");
    expect(mocks.toast.error).toHaveBeenCalledWith("Could not respond to the permission request.");
    expect((screen.getByTestId("permission-approve") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByText("Try again"));
    await waitFor(() => expect(mocks.request).toHaveBeenCalledTimes(2));
    expect(mocks.request.mock.calls[1][1]).toEqual(mocks.request.mock.calls[0][1]);
    await waitFor(() => expect(screen.queryByTestId("permission-answer")).toBeNull());
  });

  it("collapses with the no-longer-available toast on a stale response", async () => {
    mocks.request.mockRejectedValue(new Error("permission_stale"));
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    fireEvent.click(await screen.findByTestId("permission-approve"));
    await waitFor(() => expect(screen.queryByTestId("permission-answer")).toBeNull());
    expect(mocks.toast.warning).toHaveBeenCalledWith(
      "This permission request is no longer available.",
    );
  });

  it("collapses when the expand read finds no permission and the manager has not engaged", async () => {
    const { button } = await renderPermissionItem();
    mocks.readRelay.mockResolvedValue(ok(NONE));
    fireEvent.click(button);
    await waitFor(() => expect(screen.queryByTestId("permission-answer")).toBeNull());
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
  });

  it("collapses on a refused expand read and offers no retry", async () => {
    const { button } = await renderPermissionItem();
    mocks.readRelay.mockResolvedValue({ kind: "refused" });
    fireEvent.click(button);
    await waitFor(() => expect(screen.queryByTestId("permission-answer")).toBeNull());
    expect(screen.queryByTestId("needs-you-answer-here")).toBeNull();
  });

  it("collapses on a manual collapse through Answer here", async () => {
    const { button } = await renderPermissionItem();
    fireEvent.click(button);
    await screen.findByTestId("permission-answer");
    fireEvent.click(button);
    expect(screen.queryByTestId("permission-answer")).toBeNull();
  });
});

describe("question card", () => {
  async function expandQuestion() {
    mocks.readRelay.mockResolvedValue(ok(clarificationRelay()));
    render(panel([question("t-1", "clarification")]));
    fireEvent.click(await screen.findByTestId("needs-you-answer-here"));
    await screen.findByTestId("clarification-panel-stub");
  }

  it("renders the shared panel with the Inbox row's props and no late-answer recovery", async () => {
    await expandQuestion();
    const props = mocks.panelProps.current!;
    expect(props.pending).toBe(true);
    expect(props.maxHeightVh).toBe(50);
    expect(props.onLateAnswer).toBeUndefined();
    expect(props.lateAnswerState).toBeUndefined();
    expect(typeof props.onResolved).toBe("function");
    expect((props.shortcutScopeRef as { current: HTMLElement | null }).current).toBe(
      screen.getByTestId("needs-you-item-t-1"),
    );
  });

  async function outcome(value: ClarificationOutcome) {
    await act(async () =>
      (mocks.panelProps.current!.onOutcome as (o: ClarificationOutcome) => void)(value),
    );
  }

  it("collapses without a toast when this caller recorded the answer", async () => {
    await expandQuestion();
    await outcome({ kind: "resolved", claimedByThisCaller: true, status: "answered" });
    expect(screen.queryByTestId("clarification-panel-stub")).toBeNull();
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("collapses with the Inbox notice when another caller rejected it", async () => {
    await expandQuestion();
    await outcome({ kind: "resolved", claimedByThisCaller: false, status: "rejected" });
    expect(screen.queryByTestId("clarification-panel-stub")).toBeNull();
    expect(mocks.toast).toHaveBeenCalledWith(expect.stringContaining("Which one?"));
  });

  it("collapses with the no-longer-active notice", async () => {
    await expandQuestion();
    await outcome({ kind: "no_longer_active" });
    expect(screen.queryByTestId("clarification-panel-stub")).toBeNull();
    expect(mocks.toast).toHaveBeenCalledTimes(1);
  });

  it("stays expanded on a failed submit", async () => {
    await expandQuestion();
    await outcome({ kind: "submission_failed" });
    expect(screen.getByTestId("clarification-panel-stub")).toBeTruthy();
  });
});

describe("held items", () => {
  it("keeps an expanded item in place after it leaves the live list, until it collapses", async () => {
    mocks.readRelay.mockResolvedValue(ok(permissionRelay()));
    const a = question("t-1");
    const b = question("t-2");
    const { rerender } = render(panel([a, b]));
    const buttons = await screen.findAllByTestId("needs-you-answer-here");
    fireEvent.click(buttons[1]);
    await screen.findByTestId("permission-answer");
    rerender(panel([a]));
    expect(screen.getByTestId("needs-you-item-t-2")).toBeTruthy();
    expect(screen.getByTestId("permission-answer")).toBeTruthy();
    fireEvent.click(screen.getAllByTestId("needs-you-answer-here")[1]);
    await waitFor(() => expect(screen.queryByTestId("needs-you-item-t-2")).toBeNull());
    expect(screen.getByTestId("needs-you-item-t-1")).toBeTruthy();
  });

  it("keeps the held card when the viewer stops being a manager", async () => {
    mocks.readRelay.mockResolvedValue(ok(permissionRelay()));
    const a = question("t-1");
    const { rerender } = render(panel([a]));
    fireEvent.click(await screen.findByTestId("needs-you-answer-here"));
    await screen.findByTestId("permission-answer");
    const reads = mocks.readRelay.mock.calls.length;
    rerender(panel([a], false));
    expect(screen.getByTestId("permission-answer")).toBeTruthy();
    expect(mocks.readRelay.mock.calls.length).toBe(reads);
  });
});
