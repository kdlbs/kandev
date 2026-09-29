import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchSessionMock = vi.hoisted(() => vi.fn());
const queueMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: (...args: unknown[]) => fetchSessionMock(...args),
}));
vi.mock("@/lib/api/domains/queue-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/queue-api")>();
  return { ...actual, queueMessage: (...args: unknown[]) => queueMock(...args) };
});
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: toastMock }) }));

import { QueueAdmissionError, QueueFullError } from "@/lib/api/domains/queue-api";
import { SendBackForm, sessionAcceptsMessage } from "./send-back-form";

function session(state = "IDLE", withIncarnation = true) {
  return {
    session: { id: "s-1", state, queue_incarnation_id: withIncarnation ? "inc-1" : undefined },
  };
}

async function renderForm(onClose = vi.fn()) {
  render(<SendBackForm taskId="t-1" sessionId="s-1" label="KAN-1" onClose={onClose} />);
  await act(async () => {});
  return onClose;
}

function type(text: string) {
  fireEvent.change(screen.getByRole("textbox"), { target: { value: text } });
}

async function send() {
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
  });
}

beforeEach(() => {
  fetchSessionMock.mockReset().mockResolvedValue(session());
  queueMock.mockReset().mockResolvedValue({});
  toastMock.mockReset();
});
afterEach(cleanup);

describe("sessionAcceptsMessage", () => {
  it("accepts the live and completed states only", () => {
    for (const s of ["STARTING", "RUNNING", "IDLE", "WAITING_FOR_INPUT", "COMPLETED"]) {
      expect(sessionAcceptsMessage(s)).toBe(true);
    }
    for (const s of ["CREATED", "FAILED", "CANCELLED", undefined]) {
      expect(sessionAcceptsMessage(s)).toBe(false);
    }
  });
});

// eslint-disable-next-line max-lines-per-function -- one describe groups the form scenarios
describe("SendBackForm", () => {
  it("queues the trimmed note with the session identity and closes on success", async () => {
    const onClose = await renderForm();
    type("  fix the flaky test  ");
    await send();
    expect(queueMock).toHaveBeenCalledTimes(1);
    expect(queueMock.mock.calls[0][0]).toMatchObject({
      session_id: "s-1",
      session_incarnation_id: "inc-1",
      task_id: "t-1",
      content: "fix the flaky test",
    });
    expect(queueMock.mock.calls[0][0].client_queue_id).toBeTruthy();
    expect(toastMock).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Sent to KAN-1", variant: "success" }),
    );
    expect(onClose).toHaveBeenCalled();
  });

  it("blocks an empty note and a note over the limit, counting code points", async () => {
    await renderForm();
    const button = screen.getByRole("button", { name: "Send" }) as HTMLButtonElement;
    type("   ");
    expect(button.disabled).toBe(true);
    type("😀".repeat(4000));
    expect(screen.getByText("4000 / 4000")).not.toBeNull();
    expect(button.disabled).toBe(false);
    type("😀".repeat(4001));
    expect(button.disabled).toBe(true);
  });

  it("locks against a second submit while the send is in flight", async () => {
    let resolve!: (v: unknown) => void;
    queueMock.mockReturnValue(new Promise((res) => (resolve = res)));
    await renderForm();
    type("note");
    const form = screen.getByRole("textbox").closest("form")!;
    await act(async () => {
      fireEvent.submit(form);
      fireEvent.submit(form);
    });
    expect(queueMock).toHaveBeenCalledTimes(1);
    await act(async () => resolve({}));
  });

  it("keeps the form and the draft on failure and reuses the queue id for the same note", async () => {
    queueMock.mockRejectedValueOnce(new Error("boom raw text")).mockResolvedValueOnce({});
    const onClose = await renderForm();
    type("same note");
    await send();
    expect(screen.getByRole("alert").textContent).toBe("Could not send. Try again.");
    expect(screen.queryByText(/boom raw/)).toBeNull();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("same note");
    expect(onClose).not.toHaveBeenCalled();
    await send();
    expect(queueMock.mock.calls[1][0].client_queue_id).toBe(
      queueMock.mock.calls[0][0].client_queue_id,
    );
    expect(onClose).toHaveBeenCalled();
  });

  it("uses a new queue id when the note changed after a failure", async () => {
    queueMock.mockRejectedValueOnce(new Error("x")).mockResolvedValueOnce({});
    await renderForm();
    type("first");
    await send();
    type("second");
    await send();
    expect(queueMock.mock.calls[1][0].client_queue_id).not.toBe(
      queueMock.mock.calls[0][0].client_queue_id,
    );
  });

  it("gives a deliberate re-send after success a fresh queue id", async () => {
    await renderForm();
    type("again");
    await send();
    cleanup();
    await renderForm();
    type("again");
    await send();
    expect(queueMock.mock.calls[1][0].client_queue_id).not.toBe(
      queueMock.mock.calls[0][0].client_queue_id,
    );
  });

  it("resets the queue id on success within the same instance", async () => {
    await renderForm();
    type("again");
    await send();
    await send();
    expect(queueMock).toHaveBeenCalledTimes(2);
    expect(queueMock.mock.calls[1][0].client_queue_id).not.toBe(
      queueMock.mock.calls[0][0].client_queue_id,
    );
  });

  it("toasts a late success without touching the closed form", async () => {
    let resolveSend: (value: unknown) => void = () => {};
    queueMock.mockReturnValueOnce(new Promise((resolve) => (resolveSend = resolve)));
    const onClose = vi.fn();
    const view = render(
      <SendBackForm taskId="t-1" sessionId="s-1" label="KAN-1" onClose={onClose} />,
    );
    await act(async () => {});
    type("late");
    await send();
    view.unmount();
    await act(async () => resolveSend({}));
    expect(onClose).not.toHaveBeenCalled();
    expect(toastMock).toHaveBeenCalledTimes(1);
  });

  it("stays silent when a send fails after the form closed", async () => {
    let rejectSend: (reason: unknown) => void = () => {};
    queueMock.mockReturnValueOnce(new Promise((_, reject) => (rejectSend = reject)));
    const view = render(
      <SendBackForm taskId="t-1" sessionId="s-1" label="KAN-1" onClose={vi.fn()} />,
    );
    await act(async () => {});
    type("late");
    await send();
    view.unmount();
    await act(async () => rejectSend(new Error("x")));
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("ignores a stale session read that lands after the session changed", async () => {
    let resolveStale: (value: unknown) => void = () => {};
    fetchSessionMock.mockReturnValueOnce(new Promise((resolve) => (resolveStale = resolve)));
    fetchSessionMock.mockResolvedValueOnce(session());
    const view = render(
      <SendBackForm taskId="t-1" sessionId="s-1" label="KAN-1" onClose={vi.fn()} />,
    );
    view.rerender(<SendBackForm taskId="t-1" sessionId="s-2" label="KAN-1" onClose={vi.fn()} />);
    await act(async () => {});
    await act(async () => resolveStale(session("FAILED")));
    expect(screen.queryByRole("alert")).toBeNull();
    type("ok");
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("disables the note and Send while a send is in flight", async () => {
    let resolveSend: (value: unknown) => void = () => {};
    queueMock.mockReturnValueOnce(new Promise((resolve) => (resolveSend = resolve)));
    await renderForm();
    type("busy");
    await send();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => resolveSend({}));
  });

  it("maps queue-full and admission errors to translated copy", async () => {
    queueMock.mockRejectedValueOnce(new QueueFullError(10, 10));
    await renderForm();
    type("n");
    await send();
    expect(screen.getByRole("alert").textContent).toBe("This task's message queue is full.");
    queueMock.mockRejectedValueOnce(new QueueAdmissionError("session-unavailable"));
    await send();
    expect(screen.getByRole("alert").textContent).not.toContain("session-unavailable");
  });

  it("refuses a session that does not accept messages or has no incarnation", async () => {
    fetchSessionMock.mockResolvedValue(session("FAILED"));
    await renderForm();
    expect(screen.getByRole("alert").textContent).toBe("This task is not accepting messages.");
    type("n");
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(true);
    cleanup();
    fetchSessionMock.mockResolvedValue(session("IDLE", false));
    await renderForm();
    expect(screen.getByRole("alert").textContent).toBe("This task is not accepting messages.");
  });

  it("offers a retry after a failed read and disables Send until it lands", async () => {
    fetchSessionMock.mockRejectedValueOnce(new Error("net")).mockResolvedValueOnce(session());
    await renderForm();
    expect(screen.getByRole("alert").textContent).toContain("Could not check the task");
    type("n");
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    });
    expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("discards on Cancel and on Escape without sending", async () => {
    const onClose = await renderForm();
    type("n");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(2);
    expect(queueMock).not.toHaveBeenCalled();
  });
});
