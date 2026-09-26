import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationSaveQueryDialog } from "./integration-save-query-dialog";

afterEach(cleanup);

describe("IntegrationSaveQueryDialog", () => {
  it("uses the shared native save-query anatomy and returns the selected defaults", () => {
    const onSave = vi.fn();
    render(
      <IntegrationSaveQueryDialog
        open
        onOpenChange={() => undefined}
        description="Save this Bitbucket pull-request search."
        suggestedLabel="Needs my review"
        query="state:open reviewer:me"
        repositoryId="repo-a"
        repositoryOptions={[{ value: "repo-a", label: "acme/widgets" }]}
        onSave={onSave}
      />,
    );

    expect(screen.getByRole("dialog", { name: "Save query" })).toBeTruthy();
    expect(screen.getByText("state:open reviewer:me")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Review queue" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(onSave).toHaveBeenCalledWith("Review queue", "repo-a");
  });

  it("disables save for a blank label", () => {
    render(
      <IntegrationSaveQueryDialog
        open
        onOpenChange={() => undefined}
        description="Save search."
        suggestedLabel=""
        onSave={() => undefined}
      />,
    );

    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
