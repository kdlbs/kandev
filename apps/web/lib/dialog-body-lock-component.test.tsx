import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { Dialog } from "@kandev/ui/dialog";
import { AlertDialog } from "@kandev/ui/alert-dialog";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Dialog body-lock recovery subscription", () => {
  it("shares one visibility listener across mounted dialog roots", () => {
    const addEventListener = vi.spyOn(document, "addEventListener");
    const removeEventListener = vi.spyOn(document, "removeEventListener");

    const view = render(
      <>
        <Dialog />
        <Dialog />
      </>,
    );

    expect(
      addEventListener.mock.calls.filter(([type]) => type === "visibilitychange"),
    ).toHaveLength(1);

    view.unmount();

    expect(
      removeEventListener.mock.calls.filter(([type]) => type === "visibilitychange"),
    ).toHaveLength(1);
  });
});

it("shares a foreground close-recovery listener across alert dialog roots", () => {
  const addEventListener = vi.spyOn(document, "addEventListener");
  const removeEventListener = vi.spyOn(document, "removeEventListener");
  const view = render(
    <>
      <AlertDialog />
      <AlertDialog />
    </>,
  );
  expect(addEventListener.mock.calls.filter(([type]) => type === "visibilitychange")).toHaveLength(
    1,
  );
  view.unmount();
  expect(
    removeEventListener.mock.calls.filter(([type]) => type === "visibilitychange"),
  ).toHaveLength(1);
});

it("unmounting an unrelated closed alert root preserves another modal's body lock", () => {
  const view = render(<AlertDialog />);
  document.body.style.pointerEvents = "none";
  view.unmount();
  expect(document.body.style.pointerEvents).toBe("none");
  document.body.style.removeProperty("pointer-events");
});
