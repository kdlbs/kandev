import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import {
  ChangeRequestPartialStatus,
  PRBranchSummary,
  PRDescriptionField,
  PRTitleField,
} from "./vcs-dialog-fields";

afterEach(cleanup);

describe.each([
  ["gitlab", "Merge Request", "MR"],
  ["github", "Pull Request", "PR"],
])("change request fields for %s", (provider, longName, shortName) => {
  it("uses provider terminology in shared change-request fields", () => {
    const terminology = getChangeRequestTerminology(provider);
    render(
      <TooltipProvider>
        <PRTitleField
          prTitle="Title"
          onPrTitleChange={() => {}}
          onGenerateTitle={() => {}}
          isGeneratingTitle={false}
          isUtilityConfigured
          terminology={terminology}
        />
        <PRDescriptionField
          prBody="Body"
          onPrBodyChange={() => {}}
          onGenerateDescription={() => {}}
          isGeneratingDescription={false}
          isUtilityConfigured
          terminology={terminology}
        />
        <PRBranchSummary displayBranch="feature" baseBranch="main" terminology={terminology} />
        <ChangeRequestPartialStatus terminology={terminology} />
      </TooltipProvider>,
    );

    const titleInput = screen.getByLabelText(`${longName} title`) as HTMLInputElement;
    expect(titleInput.placeholder).toBe(`${longName} title...`);
    expect(
      screen.getByRole("button", { name: `Generate ${shortName} title with AI` }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: `Generate ${shortName} description with AI` }),
    ).toBeTruthy();
    expect(screen.getByText(new RegExp(`Creating ${shortName} from`))).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe(
      `Branch was pushed; retry ${longName.toLowerCase()} creation.`,
    );
  });
});
