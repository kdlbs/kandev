import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IntegrationRepositoryFilter } from "./integration-repository-filter";

afterEach(cleanup);

describe("IntegrationRepositoryFilter", () => {
  it("searches provider-neutral repository options and returns the selected identity", async () => {
    const onValueChange = vi.fn();
    render(
      <TooltipProvider>
        <IntegrationRepositoryFilter
          value=""
          onValueChange={onValueChange}
          options={[
            { value: "repo-1", label: "acme/api" },
            { value: "repo-2", label: "acme/web" },
          ]}
          ariaLabel="Filter pull requests by repository"
          testId="repository-filter"
          dropdownTestId="repository-filter-dropdown"
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByText("All repositories"));
    // The unset props fall back to the `integrations:*` catalog, which spells the
    // placeholder with a real ellipsis. Both app consumers pass all three props,
    // so this default is only ever rendered here.
    fireEvent.change(await screen.findByPlaceholderText("Filter repositories…"), {
      target: { value: "web" },
    });
    fireEvent.click(await screen.findByRole("option", { name: "acme/web" }));

    expect(onValueChange).toHaveBeenCalledWith("repo-2");
  });

  it("returns an empty value from the all-repositories option", async () => {
    const onValueChange = vi.fn();
    render(
      <TooltipProvider>
        <IntegrationRepositoryFilter
          value="repo-1"
          onValueChange={onValueChange}
          options={[{ value: "repo-1", label: "acme/api" }]}
          ariaLabel="Filter pull requests by repository"
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByText("acme/api"));
    fireEvent.click(await screen.findByRole("option", { name: "All repositories" }));

    expect(onValueChange).toHaveBeenCalledWith("");
  });
});
