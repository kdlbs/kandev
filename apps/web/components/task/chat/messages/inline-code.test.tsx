import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { InlineCode } from "./inline-code";

const { copyMock } = vi.hoisted(() => ({
  copyMock: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copy: copyMock }),
}));

describe("InlineCode", () => {
  beforeEach(() => {
    copyMock.mockClear();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the inline code content", () => {
    render(<InlineCode>npm install</InlineCode>);
    expect(screen.getByText("npm install")).toBeDefined();
  });

  it("portals the hover tooltip to document.body so overflow-hidden ancestors cannot clip it", () => {
    // Mirrors the user-message bubble: content wrapped in nested overflow-hidden
    // layers that previously clipped the in-flow absolute tooltip.
    const { container } = render(
      <div className="overflow-hidden">
        <div className="overflow-hidden">
          <InlineCode>git status</InlineCode>
        </div>
      </div>,
    );

    fireEvent.mouseEnter(screen.getByText("git status"));

    const tooltip = screen.getByRole("tooltip");
    expect(tooltip.textContent).toBe("Copy to clipboard");
    // The tooltip must escape the clipping wrapper: it lives on document.body,
    // not inside the overflow-hidden container.
    expect(container.contains(tooltip)).toBe(false);
    expect(document.body.contains(tooltip)).toBe(true);
  });

  it("hides the tooltip when the pointer leaves", () => {
    render(<InlineCode>ls</InlineCode>);
    const code = screen.getByText("ls");

    fireEvent.mouseEnter(code);
    expect(screen.queryByRole("tooltip")).not.toBeNull();

    fireEvent.mouseLeave(code);
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("copies the content and shows a Copied! confirmation on click", async () => {
    render(<InlineCode>echo hi</InlineCode>);

    await act(async () => {
      fireEvent.click(screen.getByText("echo hi"));
    });

    expect(copyMock).toHaveBeenCalledWith("echo hi");
    expect(screen.getByRole("tooltip").textContent).toBe("Copied!");
  });

  it("wires the trigger to the tooltip via aria-describedby only while visible", () => {
    render(<InlineCode>pwd</InlineCode>);
    const code = screen.getByText("pwd");

    expect(code.getAttribute("aria-describedby")).toBeNull();

    fireEvent.mouseEnter(code);
    const tooltip = screen.getByRole("tooltip");
    expect(code.getAttribute("aria-describedby")).toBe(tooltip.id);
    expect(tooltip.id).not.toBe("");

    fireEvent.mouseLeave(code);
    expect(code.getAttribute("aria-describedby")).toBeNull();
  });

  it("dismisses the tooltip on scroll so it cannot drift from the anchored code", () => {
    render(<InlineCode>cd</InlineCode>);

    fireEvent.mouseEnter(screen.getByText("cd"));
    expect(screen.queryByRole("tooltip")).not.toBeNull();

    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });

    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
