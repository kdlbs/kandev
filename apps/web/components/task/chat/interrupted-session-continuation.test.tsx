import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { InterruptedSessionContinuation } from "./interrupted-session-continuation";
const mocks = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request: mocks.request }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
const observed = {
  task_id: "task",
  session_id: "session",
  outcome: "uncertain" as const,
  recovery_revision: 1,
  allowed_actions: ["resume_interrupted"],
  recovery_identity: {
    submission_id: "old",
    stream_id: "stream",
    incarnation_id: "inc",
    harness_generation: 1,
    prompt_generation: 1,
  },
};
beforeEach(() => {
  window.sessionStorage.clear();
  vi.clearAllMocks();
});
afterEach(cleanup);
it("announces progress with a stable action name and resets for a new interruption", async () => {
  let complete!: (value: unknown) => void;
  mocks.request.mockImplementation(async (action) =>
    action === "session.recover"
      ? observed
      : new Promise((resolve) => {
          complete = resolve;
        }),
  );
  const { rerender } = render(<InterruptedSessionContinuation observed={observed} />);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "First instruction" } });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "task:interruptedRecoveryResume" }));
  expect(
    (screen.getByRole("button", { name: "task:interruptedRecoveryResume" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.getByRole("status").textContent).toBe("task:resuming");
  await vi.waitFor(() => expect(complete).toBeTypeOf("function"));
  const next = {
    ...observed,
    recovery_revision: 3,
    recovery_identity: {
      ...observed.recovery_identity,
      submission_id: "next",
      harness_generation: 2,
    },
  };
  rerender(<InterruptedSessionContinuation observed={next} />);
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(false);
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(false);
  await act(async () => {
    complete({ completed: 1, results: [{ ...observed, outcome: "continued" }] });
  });
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  expect(screen.queryByTestId("interrupted-recovery-result")).toBeNull();
});
