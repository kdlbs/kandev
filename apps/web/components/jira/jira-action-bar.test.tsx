import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { JiraActionBar } from "./jira-action-bar";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});
it("keeps Jira removal in a phone sheet without testing or removing on Cancel", async () => {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
  const onDelete = vi.fn();
  const onTest = vi.fn();
  render(
    <JiraActionBar
      workspaceId="workspace-a"
      testing={false}
      loading={false}
      hasConfig
      disableTest={false}
      onTest={onTest}
      onDelete={onDelete}
    />,
  );
  const trigger = screen.getByTestId("jira-delete-button");
  fireEvent.click(trigger);
  const sheet = screen.getByRole("dialog");
  expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
  expect(trigger.isConnected).toBe(true);
  fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(onDelete).not.toHaveBeenCalled();
  expect(onTest).not.toHaveBeenCalled();
  fireEvent.click(trigger);
  fireEvent.click(screen.getByTestId("jira-remove-confirm"));
  await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1));
});
