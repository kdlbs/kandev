import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { vi, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { MessageAction } from "@/components/task/chat/types";
import { ActionButton } from "./action-message-actions";
import type { TaskSession } from "@/lib/types/http";

const getSubtaskCountMock = vi.hoisted(() => vi.fn());
const getTaskDeletePreflightMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal()),
  getSubtaskCount: getSubtaskCountMock,
  getTaskDeletePreflight: getTaskDeletePreflightMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

describe("ActionButton task deletion", () => {
  it("opens the shared discard-consent dialog", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });
    getTaskDeletePreflightMock.mockResolvedValue({
      requires_discard_consent: false,
      confirmation_id: "delete-confirmation-1",
    });
    const action: MessageAction = {
      type: "delete_task",
      label: "Delete task",
      variant: "destructive",
      test_id: "message-delete-task-button",
    };

    render(
      <StateProvider>
        <ActionButton action={action} messageTaskId="task-1" />
      </StateProvider>,
    );

    fireEvent.click(screen.getByTestId("message-delete-task-button"));
    expect(await screen.findByRole("alertdialog")).toBeTruthy();
    await waitFor(() =>
      expect(getTaskDeletePreflightMock).toHaveBeenCalledWith(["task-1"], false, false),
    );
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement).disabled).toBe(
        false,
      ),
    );
    expect(screen.queryByTestId("delete-discard-worktree-checkbox")).toBeNull();
  });
});

describe("stall cancellation", () => {
  it("keeps hydrated cancellation busy and prevents another request", () => {
    const action: MessageAction = {
      type: "ws_request",
      label: "Cancel turn",
      test_id: "stall-cancel-turn-button",
      params: { method: "agent.cancel", payload: { session_id: "s1" } },
    };
    render(
      <StateProvider
        initialState={{
          taskSessions: {
            items: {
              s1: {
                id: "s1",
                task_id: "t1",
                state: "RUNNING",
                cancellation_pending: true,
              } as TaskSession,
            },
          },
        }}
      >
        <ActionButton action={action} compact />
      </StateProvider>,
    );
    expect((screen.getByTestId("stall-cancel-turn-button") as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.getByRole("status", { name: "Cancelling..." })).toBeTruthy();
  });
});
