import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MessageEditForm } from "./message-edit-form";

afterEach(cleanup);

function form(text: string) {
  return (
    <MessageEditForm
      text={text}
      busy={false}
      serverError={null}
      onApprove={vi.fn()}
      onCancel={vi.fn()}
    />
  );
}

describe("MessageEditForm", () => {
  it("gives each open form its own textarea id and label target", () => {
    render(
      <>
        {form("first")}
        {form("second")}
      </>,
    );
    const areas = screen.getAllByRole("textbox") as HTMLTextAreaElement[];
    expect(areas).toHaveLength(2);
    expect(areas[0].id).not.toBe(areas[1].id);
    const labels = screen.getAllByText(/message/i).filter((el) => el.tagName === "LABEL");
    expect(labels.map((l) => l.getAttribute("for"))).toEqual(areas.map((a) => a.id));
  });
});
