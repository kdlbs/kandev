import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { Dialog } from "@kandev/ui/dialog";

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
