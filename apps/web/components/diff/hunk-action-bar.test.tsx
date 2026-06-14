import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { HunkActionBar } from "./hunk-action-bar";

describe("HunkActionBar", () => {
  afterEach(() => {
    cleanup();
  });

  it("clears loading when revert resolves without unmounting the hunk", async () => {
    const onRevert = vi.fn().mockResolvedValue(undefined);

    render(
      <HunkActionBar
        changeBlockId="cb-1"
        onRevert={onRevert}
        onMouseEnter={() => {}}
        onMouseLeave={() => {}}
      />,
    );

    const button = screen.getByRole("button", { name: "Undo" });
    fireEvent.click(button);

    await waitFor(() => expect(onRevert).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
  });
});
