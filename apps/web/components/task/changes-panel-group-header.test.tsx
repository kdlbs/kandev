import { createRef } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ChangesPanelGroupHeader } from "./changes-panel-group-header";

it("forwards native button props and ref while preserving disclosure activation", () => {
  const ref = createRef<HTMLButtonElement>();
  const onContextMenu = vi.fn();
  const onPointerDown = vi.fn();
  const onClick = vi.fn();
  const onToggle = vi.fn();

  render(
    <ChangesPanelGroupHeader
      ref={ref}
      label="Outside repositories"
      count={2}
      collapsed={false}
      onToggle={onToggle}
      testId="group-header"
      className="consumer-class"
      aria-label="Outside repositories 2"
      data-consumer-prop="forwarded"
      onContextMenu={onContextMenu}
      onPointerDown={onPointerDown}
      onClick={onClick}
    />,
  );

  const button = screen.getByRole("button", { name: "Outside repositories 2" });
  expect(ref.current).toBe(button);
  expect(button.classList.contains("consumer-class")).toBe(true);
  expect(button.getAttribute("data-consumer-prop")).toBe("forwarded");

  fireEvent.contextMenu(button);
  fireEvent.pointerDown(button);
  fireEvent.click(button);

  expect(onContextMenu).toHaveBeenCalledOnce();
  expect(onPointerDown).toHaveBeenCalledOnce();
  expect(onClick).toHaveBeenCalledOnce();
  expect(onToggle).toHaveBeenCalledOnce();
});
