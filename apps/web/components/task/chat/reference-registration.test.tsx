import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  changeEntityReferenceAssociationMode,
  createEntityReferenceSelection,
  ReferenceRegistrationControl,
} from "./reference-registration";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

afterEach(() => cleanup());

const pullRequest: EntityReference = {
  version: 1,
  ref: "mention:v1:github:pull_request:github.com%2Facme%2Fapp:PR_kwDOAA",
  provider: "github",
  kind: "pull_request",
  id: "PR_kwDOAA",
  key: "acme/app#12",
  title: "Add authentication",
  url: "https://github.com/acme/app/pull/12",
  scope: "github.com/acme/app",
};

describe("entity reference registration identity", () => {
  it("assigns a fresh selection ID and defaults supported pull requests to link on send", () => {
    expect(createEntityReferenceSelection(pullRequest, "selection-a")).toMatchObject({
      selection_id: "selection-a",
      association_mode: "link_on_send",
    });
  });

  it("keeps a selected change request context-only when admission cannot create registrations", () => {
    expect(createEntityReferenceSelection(pullRequest, "selection-queue", false)).toMatchObject({
      selection_id: "selection-queue",
      association_mode: "context_only",
    });
  });

  it("shows a fixed context-only chip when the current admission path cannot register", () => {
    render(
      <ReferenceRegistrationControl
        referenceLabel="#acme/app#12"
        mode="context_only"
        linkingEnabled={false}
        onModeChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("reference-context-only-chip").textContent).toContain(
      "task:referenceContextOnly",
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByText("task:referenceLinkOnSend")).toBeNull();
  });

  it("keeps unsupported references context only", () => {
    const issue = { ...pullRequest, kind: "issue", ref: "issue-ref" };
    expect(createEntityReferenceSelection(issue, "selection-b")).toMatchObject({
      selection_id: "selection-b",
      association_mode: "context_only",
    });
  });

  it("changes context-only to link-on-send with a new selection identity", () => {
    const selected = createEntityReferenceSelection(pullRequest, "selection-first");
    const contextOnly = changeEntityReferenceAssociationMode(selected, "context_only");
    const linked = changeEntityReferenceAssociationMode(
      contextOnly,
      "link_on_send",
      "selection-second",
    );

    expect(linked.association_mode).toBe("link_on_send");
    expect(linked.selection_id).toBe("selection-second");
  });

  it("generates a valid fresh identity when a context-only selection is linked again", () => {
    const selected = createEntityReferenceSelection(pullRequest);
    const linkedAgain = changeEntityReferenceAssociationMode(
      changeEntityReferenceAssociationMode(selected, "context_only"),
      "link_on_send",
    );

    expect(selected.selection_id).toMatch(/^[0-9a-f-]{36}$/iu);
    expect(linkedAgain.selection_id).toMatch(/^[0-9a-f-]{36}$/iu);
    expect(linkedAgain.selection_id).not.toBe(selected.selection_id);
  });

  it("uses a fresh selection when explicitly linking an unlinked reference again", () => {
    const previous = {
      ...createEntityReferenceSelection(pullRequest, "selection-old"),
      registration_status: "unlinked" as const,
    };
    const next = changeEntityReferenceAssociationMode(previous, "link_on_send", "selection-new");
    expect(next.selection_id).toBe("selection-new");
    expect(next.registration_status).toBeUndefined();
  });

  it("preserves identity when the user chooses context only", () => {
    const selected = createEntityReferenceSelection(pullRequest, "selection-stable");
    expect(changeEntityReferenceAssociationMode(selected, "context_only")).toMatchObject({
      selection_id: "selection-stable",
      association_mode: "context_only",
    });
  });
});

describe("entity reference registration controls", () => {
  it("makes the trigger and choices actionable and coarse-pointer sized", () => {
    render(
      <ReferenceRegistrationControl
        referenceLabel="#acme/app#12"
        mode="context_only"
        onModeChange={vi.fn()}
      />,
    );

    const trigger = screen.getByTestId("reference-registration-trigger");
    expect(trigger.className).toContain("cursor-pointer");
    expect(trigger.className).toContain("[@media(pointer:coarse)]:min-h-11");
    fireEvent.click(trigger);

    for (const mode of ["link_on_send", "context_only"]) {
      const choice = screen.getByTestId(`reference-registration-${mode}`);
      expect(choice.className).toContain("cursor-pointer");
      expect(choice.className).toContain("[@media(pointer:coarse)]:min-h-11");
    }
  });
});
