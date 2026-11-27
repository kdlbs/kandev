import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ChangesPanelRefreshIndicator } from "./changes-panel-refresh-status";

const LOADING_STATUS = "loading" as const;
const STATUS_TEST_ID = "changes-refresh-status";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { repository?: string }) => {
      if (key === "task:loadingChanges") return "Loading changes...";
      if (key === "task:gitStatusAutomaticRetry")
        return "Changes did not refresh. Retrying automatically.";
      if (key === "task:gitStatusShowingLastAvailable")
        return "Showing the last available changes.";
      if (key === "task:gitStatusRepositoryUnavailable")
        return `${options?.repository} could not be refreshed.`;
      return key;
    },
  }),
}));

afterEach(cleanup);

describe("ChangesPanelRefreshIndicator", () => {
  it("keeps the tooltip open while the status has keyboard focus", async () => {
    render(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator
          status={LOADING_STATUS}
          hasPriorData={false}
          failedRepositories={[]}
        />
      </TooltipProvider>,
    );

    const status = screen.getByTestId(STATUS_TEST_ID);
    status.focus();

    expect(document.activeElement).toBe(status);
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip.textContent).toContain("Loading changes...");

    fireEvent.blur(status);
    await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
  });

  it("lets Escape dismiss the tooltip while its trigger remains focused", async () => {
    render(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator
          status={LOADING_STATUS}
          hasPriorData={false}
          failedRepositories={[]}
        />
      </TooltipProvider>,
    );

    const status = screen.getByTestId(STATUS_TEST_ID);
    status.focus();
    await screen.findByRole("tooltip");

    fireEvent.keyDown(status, { key: "Escape" });

    await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
    expect(document.activeElement).toBe(status);
  });

  it("announces unavailable details and preserved rows through the live region", async () => {
    render(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator
          status="unavailable"
          hasPriorData
          failedRepositories={["backend"]}
        />
      </TooltipProvider>,
    );

    const status = screen.getByTestId(STATUS_TEST_ID);
    expect(status.textContent).toContain("Changes did not refresh. Retrying automatically.");
    expect(status.textContent).toContain("Showing the last available changes.");
    expect(status.textContent).toContain("backend could not be refreshed.");
    expect(status.getAttribute("aria-label")).toBeNull();

    status.focus();
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip.textContent).toContain("Changes did not refresh. Retrying automatically.");
    expect(tooltip.textContent).toContain("Showing the last available changes.");
    expect(tooltip.textContent).toContain("backend could not be refreshed.");
  });

  it("resets focus interaction when the refresh indicator disappears", async () => {
    const props = {
      hasPriorData: false,
      failedRepositories: [] as string[],
    };
    const { rerender } = render(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator status={LOADING_STATUS} {...props} />
      </TooltipProvider>,
    );

    const firstStatus = screen.getByTestId(STATUS_TEST_ID);
    firstStatus.focus();
    await screen.findByRole("tooltip");

    rerender(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator status={null} {...props} />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId(STATUS_TEST_ID)).toBeNull();

    rerender(
      <TooltipProvider>
        <ChangesPanelRefreshIndicator status={LOADING_STATUS} {...props} />
      </TooltipProvider>,
    );

    await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
    expect(document.activeElement).not.toBe(screen.getByTestId(STATUS_TEST_ID));
  });
});
