import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ListErrorState } from "./list-error-state";

afterEach(cleanup);

describe("ListErrorState", () => {
  it("shows the message and calls retry on Try again", () => {
    const retry = vi.fn();
    render(<ListErrorState retry={retry} />);
    expect(screen.getByText("Could not load coordinators.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
