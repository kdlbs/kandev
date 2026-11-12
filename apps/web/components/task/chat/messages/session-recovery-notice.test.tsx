import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SessionRecoveryNotice } from "./session-recovery-notice";

describe("SessionRecoveryNotice", () => {
  it("renders translated informational copy without recovery controls", () => {
    render(<SessionRecoveryNotice />);

    expect(screen.getByRole("note").getAttribute("data-testid")).toBe("session-recovery-notice");
    expect(screen.getByRole("note").textContent).toBe(
      "The session was interrupted. Your conversation is restored. You can continue.",
    );
    expect(screen.queryByRole("button")).toBeNull();
  });
});
