import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { TaskWalkthrough } from "@/lib/types/http";
import { WalkthroughStepPicker } from "./walkthrough-step-picker";

const PICKER_LABEL = "Go to step";
const EXPANDED = "aria-expanded";
const DETAILS_LABEL = "2. Details (0 comments)";

const responsive = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
beforeEach(() => {
  responsive.isMobile = false;
  responsive.isFinePointer = true;
});
afterEach(cleanup);
afterEach(() => vi.restoreAllMocks());
const walkthrough = {
  steps: [{ title: "Overview" }, { title: "Details" }],
} as TaskWalkthrough;

it("opens the shared picker with keyboard selection and comment counts", () => {
  const onSelect = vi.fn();
  render(
    <WalkthroughStepPicker
      walkthrough={walkthrough}
      activeStep={0}
      counts={[1, 0]}
      onSelect={onSelect}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: PICKER_LABEL });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  expect(trigger.getAttribute(EXPANDED)).toBe("true");
  fireEvent.keyDown(screen.getByRole("option", { name: DETAILS_LABEL }), {
    key: "Enter",
  });
  expect(onSelect).toHaveBeenCalledExactlyOnceWith(1);
});

it.each(["phone", "coarse pointer"])("selects once from the %s sheet after a resize", (mode) => {
  responsive.isMobile = mode === "phone";
  responsive.isFinePointer = mode !== "coarse pointer";
  const onSelect = vi.fn();
  render(
    <WalkthroughStepPicker
      walkthrough={walkthrough}
      activeStep={0}
      counts={[1, 0]}
      onSelect={onSelect}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: PICKER_LABEL });
  fireEvent.click(trigger);
  expect(screen.getByRole("dialog", { name: PICKER_LABEL })).toBeTruthy();
  fireEvent(window, new Event("resize"));
  fireEvent.click(screen.getByRole("option", { name: DETAILS_LABEL }));
  expect(onSelect).toHaveBeenCalledExactlyOnceWith(1);
  expect(trigger.getAttribute(EXPANDED)).toBe("false");
});

function pointer(element: Element, type: string, overrides = {}) {
  const drawer = document.querySelector<HTMLElement>('[data-slot="drawer-content"]');
  if (drawer) drawer.style.transform = "none";
  vi.spyOn(element, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 200, 44));
  const event = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: 80, clientY: 20 });
  Object.defineProperties(event, {
    pointerType: { value: "touch" },
    pointerId: { value: 1 },
    isPrimary: { value: true },
    ...Object.fromEntries(Object.entries(overrides).map(([key, value]) => [key, { value }])),
  });
  fireEvent(element, event);
}

function renderPhonePicker() {
  responsive.isMobile = true;
  const onSelect = vi.fn();
  const view = render(
    <WalkthroughStepPicker
      walkthrough={walkthrough}
      activeStep={0}
      counts={[0, 0]}
      onSelect={onSelect}
    />,
  );
  return { ...view, onSelect, trigger: screen.getByRole("combobox", { name: PICKER_LABEL }) };
}

// @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4
it("opens and selects from completed touches without compatibility clicks", () => {
  const { trigger, onSelect } = renderPhonePicker();
  pointer(trigger, "pointerdown");
  pointer(trigger, "pointerup");
  expect(trigger.getAttribute(EXPANDED)).toBe("true");
  const option = screen.getByRole("option", { name: DETAILS_LABEL });
  pointer(option, "pointerdown");
  pointer(option, "pointerup");
  expect(onSelect).toHaveBeenCalledExactlyOnceWith(1);
  expect(trigger.getAttribute(EXPANDED)).toBe("false");
  // Safari can retarget its later click after the selected row unmounts.
  fireEvent.click(trigger, { detail: 1 });
  expect(trigger.getAttribute(EXPANDED)).toBe("false");
  fireEvent.click(trigger, { detail: 0 });
  expect(trigger.getAttribute(EXPANDED)).toBe("true");
});

it.each(["scroll", "cancel", "outside", "long press", "secondary touch"])(
  "does not select a step after %s",
  (gesture) => {
    const { trigger, onSelect } = renderPhonePicker();
    fireEvent.click(trigger);
    const option = screen.getByRole("option", { name: DETAILS_LABEL });
    pointer(option, "pointerdown", { timeStamp: 100 });
    if (gesture === "scroll") pointer(option, "pointermove", { clientY: 40 });
    if (gesture === "cancel") pointer(option, "pointercancel");
    if (gesture === "secondary touch")
      pointer(option, "pointerdown", { pointerId: 2, isPrimary: false });
    pointer(option, "pointerup", {
      clientX: gesture === "outside" ? 201 : 80,
      timeStamp: gesture === "long press" ? 700 : 200,
    });
    fireEvent.click(option, { detail: 1 });
    expect(onSelect).not.toHaveBeenCalled();
    expect(trigger.getAttribute(EXPANDED)).toBe("true");
    pointer(option, "pointerdown", { pointerId: 3 });
    pointer(option, "pointerup", { pointerId: 3 });
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(1);
  },
);

it("preserves mouse activation after a touch and clears click guards on unmount", () => {
  const { trigger, unmount } = renderPhonePicker();
  pointer(trigger, "pointerdown");
  pointer(trigger, "pointerup");
  fireEvent.keyDown(document.activeElement!, { key: "Escape" });
  pointer(trigger, "pointerdown", { pointerType: "mouse" });
  pointer(trigger, "pointerup", { pointerType: "mouse" });
  fireEvent.click(trigger, { detail: 1 });
  expect(trigger.getAttribute(EXPANDED)).toBe("true");
  const option = screen.getByRole("option", { name: DETAILS_LABEL });
  pointer(option, "pointerdown");
  pointer(option, "pointerup");
  unmount();
  const outsideClick = vi.fn();
  const view = render(<button onClick={outsideClick}>Outside</button>);
  fireEvent.click(view.getByRole("button"), { detail: 1 });
  expect(outsideClick).toHaveBeenCalledOnce();
});
