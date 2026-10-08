import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const addMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return { ...actual, addStandingOrder: (...args: unknown[]) => addMock(...args) };
});

import { AddStandingOrderDialog } from "./add-standing-order-dialog";

const RULE_TEXT = "Prefer small cards.";
const ORDER = {
  id: "o-1",
  number: 1,
  text: RULE_TEXT,
  created_at: "2026-09-12T00:00:00Z",
  created_by: "u",
  retired_at: null,
  last_applied_at: null,
};

function renderDialog(props: Partial<React.ComponentProps<typeof AddStandingOrderDialog>> = {}) {
  const onAdded = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <AddStandingOrderDialog
      workspaceId="ws-1"
      coordinatorId="c-1"
      open
      onOpenChange={onOpenChange}
      onAdded={onAdded}
      {...props}
    />,
  );
  return { onAdded, onOpenChange };
}

function type(text: string) {
  fireEvent.change(screen.getByRole("textbox"), { target: { value: text } });
}

beforeEach(() => addMock.mockReset());
afterEach(cleanup);

describe("AddStandingOrderDialog", () => {
  it("disables Add for empty and over-500 text and counts code points, not UTF-16 units", () => {
    renderDialog();
    const save = screen.getByRole("button", { name: "Add" });
    expect((save as HTMLButtonElement).disabled).toBe(true);
    type("   ");
    expect((save as HTMLButtonElement).disabled).toBe(true);
    type("😀".repeat(500));
    expect(screen.getByText("500 / 500")).toBeTruthy();
    expect((save as HTMLButtonElement).disabled).toBe(false);
    type("😀".repeat(501));
    expect((save as HTMLButtonElement).disabled).toBe(true);
  });

  it("posts the trimmed text with the source proposal id, reports the order and closes", async () => {
    addMock.mockResolvedValueOnce(ORDER);
    const { onAdded, onOpenChange } = renderDialog({
      initialText: RULE_TEXT,
      sourceProposalId: "p-1",
    });
    type("  Prefer small cards.  ");
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => expect(onAdded).toHaveBeenCalledWith(ORDER));
    expect(addMock).toHaveBeenCalledWith("ws-1", "c-1", {
      text: RULE_TEXT,
      source_proposal_id: "p-1",
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("sends one request per click while in flight", async () => {
    let resolve: (value: unknown) => void = () => {};
    addMock.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    renderDialog();
    type("Rule");
    const save = screen.getByRole("button", { name: "Add" });
    fireEvent.click(save);
    expect((save as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(save);
    expect(addMock).toHaveBeenCalledTimes(1);
    resolve(ORDER);
    await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(true));
  });

  it("shows the limit message and keeps the text on a standing_order_limit refusal", async () => {
    addMock.mockRejectedValueOnce(
      new ApiError("standing_order_limit", 400, {
        error: "standing_order_limit",
        error_code: "standing_order_limit",
      }),
    );
    const { onAdded } = renderDialog();
    type("Rule");
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(
      await screen.findByText("You can have up to 20 standing orders. Retire one to add another."),
    ).toBeTruthy();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Rule");
    expect(onAdded).not.toHaveBeenCalled();
  });

  it("shows a generic error and keeps the text on any other failure", async () => {
    addMock.mockRejectedValueOnce(new Error("boom"));
    renderDialog();
    type("Rule");
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect(await screen.findByText("Could not add the standing order. Try again.")).toBeTruthy();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Rule");
    expect((screen.getByRole("button", { name: "Add" }) as HTMLButtonElement).disabled).toBe(false);
  });
});
