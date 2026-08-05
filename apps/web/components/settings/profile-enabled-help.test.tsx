import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ProfileEnabledHelp } from "./profile-enabled-help";

afterEach(cleanup);

describe("ProfileEnabledHelp", () => {
  it("shows the profile availability explanation from the info button", async () => {
    render(
      <TooltipProvider delayDuration={0}>
        <ProfileEnabledHelp />
      </TooltipProvider>,
    );

    const infoButton = screen.getByRole("button", {
      name: "More information about profile availability",
    });
    fireEvent.click(infoButton);

    expect((await screen.findByRole("tooltip")).textContent).toContain(
      "Disabled profiles keep serving existing sessions but are not offered when creating new tasks or agents.",
    );
  });
});
