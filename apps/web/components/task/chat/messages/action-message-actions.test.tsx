import { fireEvent, render, screen } from "@testing-library/react";
import { vi, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { MessageAction } from "@/components/task/chat/types";
import { ActionButton } from "./action-message-actions";

const getSubtaskCountMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal()),
  getSubtaskCount: getSubtaskCountMock,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

describe("ActionButton task deletion", () => {
  it("opens the shared discard-consent dialog", async () => {
    getSubtaskCountMock.mockResolvedValue({ count: 0 });
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
    expect(await screen.findByTestId("delete-discard-worktree-checkbox")).toBeTruthy();
  });
});
