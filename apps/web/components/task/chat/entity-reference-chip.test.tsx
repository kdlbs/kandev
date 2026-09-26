import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import { EntityReferenceChip } from "./messages/entity-reference-chip";

afterEach(cleanup);

describe("EntityReferenceChip", () => {
  it("renders a generic plugin pull request from normalized reference data", () => {
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:plugin%3Aexample%3Areviews:pull_request:workspace-1:pull-42",
      provider: "plugin:example:reviews",
      kind: "pull_request",
      id: "pull-42",
      key: "PR-42",
      title: "Fix authentication",
      url: "https://reviews.example.test/pull-requests/42",
      scope: "workspace-1",
    };

    render(<EntityReferenceChip reference={reference} />);

    const chip = screen.getByTestId("entity-reference-chip");
    expect(chip.getAttribute("data-entity-provider")).toBe("plugin:example:reviews");
    expect(chip.getAttribute("data-entity-kind")).toBe("pull_request");
    expect(chip.getAttribute("href")).toBe(reference.url);
    expect(chip.getAttribute("target")).toBe("_blank");
    expect(chip.textContent).toContain("PR-42");
  });
});
