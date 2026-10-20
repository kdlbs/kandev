import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ContextDiff } from "./context-diff";

afterEach(cleanup);

describe("ContextDiff", () => {
  it("marks removed, added and unchanged lines with text markers and shows the key", () => {
    const { container } = render(<ContextDiff before={"keep\nold"} after={"keep\nnew"} />);
    const rows = [...container.querySelectorAll("pre > div")].map((row) => [
      row.getAttribute("data-kind"),
      row.textContent,
    ]);
    expect(rows).toEqual([
      ["context", " keep"],
      ["remove", "-old"],
      ["add", "+new"],
    ]);
    expect(screen.getByText("Context when proposed")).not.toBeNull();
    expect(screen.getByText("Proposed context")).not.toBeNull();
  });

  it("does not truncate a long line", () => {
    const long = "x".repeat(2000);
    const { container } = render(<ContextDiff before="" after={long} />);
    expect(container.textContent).toContain(long);
  });
});
