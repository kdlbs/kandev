import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { QuickChatActivityIndicator } from "./quick-chat-activity-indicator";

afterEach(cleanup);

describe("QuickChatActivityIndicator", () => {
  it.each([
    ["running", "bg-blue-500"],
    ["finished", "bg-emerald-500"],
  ] as const)("maps %s to the shared semantic bubble", (activity, colorClass) => {
    const { getByTestId } = render(<QuickChatActivityIndicator activity={activity} />);
    const indicator = getByTestId("quick-chat-activity-indicator");

    expect(indicator.getAttribute("data-state")).toBe(activity);
    expect(indicator.className).toContain(colorClass);
    expect(indicator.getAttribute("aria-hidden")).toBe("true");
  });

  it("renders nothing when there is no activity", () => {
    const { queryByTestId } = render(<QuickChatActivityIndicator activity={null} />);

    expect(queryByTestId("quick-chat-activity-indicator")).toBeNull();
  });
});
