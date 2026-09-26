import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskAutopilotToggle } from "./task-autopilot-toggle";

afterEach(cleanup);

describe("TaskAutopilotToggle", () => {
  it("shows additional autopilot guidance from the compact info control", async () => {
    render(
      <TooltipProvider delayDuration={0}>
        <TaskAutopilotToggle checked={false} onCheckedChange={vi.fn()} disabled={false} />
      </TooltipProvider>,
    );

    const infoButton = screen.getByRole("button", {
      name: "More information about autopilot",
    });
    fireEvent.click(infoButton);

    expect((await screen.findByRole("tooltip")).textContent).toContain(
      "The agent works independently and asks the parent task only when a critical decision blocks progress.",
    );
  });
});
