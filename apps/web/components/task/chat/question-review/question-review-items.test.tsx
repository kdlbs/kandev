import { createRef } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationQuestion,
  type Message,
} from "@/lib/types/http";
import { ClarificationInputOverlay } from "../clarification-input-overlay";
import { ClarificationDraftProvider } from "../clarification-draft-context";
import { clearAnswerDrafts } from "@/lib/clarification/answer-draft-cache";

const mocks = vi.hoisted(() => ({ fetchImage: vi.fn() }));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));
vi.mock("@/lib/api/domains/question-image-api", () => ({
  fetchQuestionImage: mocks.fetchImage,
  questionImageErrorCode: () => "unavailable",
}));
const mockStore = {
  getState: () => ({ updateMessage: vi.fn() }),
  subscribe: () => () => {},
};
const authState = { auth: { mode: "local", authenticated: true, user: { id: "u1" } } };
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof authState) => unknown) => selector(authState),
  useAppStoreApi: () => mockStore,
  useOptionalAppStoreApi: () => mockStore,
  useOptionalAppStore: (_selector: unknown, fallback: unknown) => fallback,
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const fetchMock = vi.fn();
const OPEN = "question-review-open";
const DIALOG = "question-review-dialog";
const MODES = "question-review-modes";
const POSITION = "question-review-item-position";
const NEXT_ITEM = "question-review-item-next";
const STAGE = "question-review-stage";
const IMAGE = "question-review-image";
const COMPARE_IMAGES = "question-review-compare-images";
const IDENTITY_BADGE = "question-review-identity-badge";
const ONION_SKIN = "Onion skin";
const FLIP = "question-review-flip";
const CHOICE_IMAGE = "question-review-choice-image";
const before = { id: "before", path: "before.png", alt: "Before" };
const after = { id: "after", path: "after.png", alt: "After" };
const billing = { id: "billing", path: "billing.png", alt: "Billing" };

function pendingMessage(question: Partial<ClarificationQuestion>): Message {
  return {
    id: "m1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-11-24T00:00:00Z",
    metadata: {
      pending_id: "p1",
      question_id: "q1",
      question_index: 0,
      question_total: 1,
      status: "pending",
      question: {
        id: "q1",
        title: "",
        prompt: "Is the header right?",
        options: [
          { option_id: "o1", label: "Approve", description: "" },
          { option_id: "o2", label: "Revise", description: "" },
        ],
        ...question,
      },
    },
  };
}

function renderOverlay(messages: Message[]) {
  const scopeRef = createRef<HTMLDivElement>();
  render(
    <ClarificationDraftProvider taskId="t1">
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={messages}
          onResolved={vi.fn()}
          onDismiss={vi.fn()}
          shortcutScopeRef={scopeRef}
        />
      </div>
    </ClarificationDraftProvider>,
  );
}

function openReview() {
  fireEvent.click(screen.getByTestId(OPEN));
  return screen.getByTestId(DIALOG);
}

beforeEach(() => {
  fetchMock.mockReset();
  mocks.fetchImage.mockReset();
  mocks.fetchImage.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: vi.fn(() => "blob:test"), revokeObjectURL: vi.fn() }),
  );
});

afterEach(() => {
  cleanup();
  clearAnswerDrafts(mockStore);
});

describe("item-by-item review", () => {
  const mixed = () =>
    pendingMessage({
      images: [before, after, billing],
      image_groups: [
        { kind: "pair", image_ids: ["before", "after"] },
        { kind: "single", image_ids: ["billing"] },
      ],
    });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.2
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.3
  it("compares a pair and shows a single alone, item by item", () => {
    renderOverlay([mixed()]);
    const dialog = openReview();
    expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 1 of 2 · Pair");
    expect(within(dialog).getByTestId(MODES)).toBeTruthy();
    // A pair's order is fixed: no pickers and no Swap.
    expect(within(dialog).queryByTestId("question-review-pick-a")).toBeNull();
    expect(within(dialog).queryByTestId("question-review-swap")).toBeNull();

    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 2 of 2 · Single");
    expect(within(dialog).queryByTestId(MODES)).toBeNull();
    expect((within(dialog).getByTestId(NEXT_ITEM) as HTMLButtonElement).disabled).toBe(true);

    const stage = within(dialog).getByTestId(STAGE);
    stage.focus();
    fireEvent.keyDown(stage, { key: "PageUp" });
    expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 1 of 2 · Pair");
    // The item's stage remounts; keyboard focus moves to the new stage.
    expect(document.activeElement).toBe(within(dialog).getByTestId(STAGE));
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-003.5
  it("keeps the comparison mode when returning to a pair from another item", () => {
    renderOverlay([mixed()]);
    const dialog = openReview();
    fireEvent.click(within(within(dialog).getByTestId(MODES)).getByText(ONION_SKIN));
    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    fireEvent.click(within(dialog).getByTestId("question-review-item-prev"));
    const onion = within(within(dialog).getByTestId(MODES)).getByText(ONION_SKIN);
    expect(onion.closest("button")?.getAttribute("data-state")).toBe("on");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.6
  it("does not fetch images again when moving between items", async () => {
    renderOverlay([mixed()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    const reads = mocks.fetchImage.mock.calls.length;
    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    fireEvent.click(within(dialog).getByTestId("question-review-item-prev"));
    await screen.findAllByTestId(IMAGE);
    expect(mocks.fetchImage.mock.calls.length).toBe(reads);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.1
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  it("shows ungrouped images as a gallery with an on-demand comparison", () => {
    renderOverlay([pendingMessage({ images: [before, after, billing] })]);
    const dialog = openReview();
    expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 1 of 3 · Image");
    expect(within(dialog).queryByTestId(MODES)).toBeNull();

    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    fireEvent.click(within(dialog).getByTestId(COMPARE_IMAGES));
    expect(within(dialog).getByTestId(MODES)).toBeTruthy();
    expect(
      within(within(dialog).getByTestId("question-review-pick-a")).getByText("2"),
    ).toBeTruthy();
    expect(
      within(within(dialog).getByTestId("question-review-pick-b")).getByText("3"),
    ).toBeTruthy();
    expect(within(dialog).getByTestId("question-review-swap")).toBeTruthy();
    expect(within(dialog).queryByTestId(POSITION)).toBeNull();

    fireEvent.click(within(dialog).getByTestId("question-review-back-to-gallery"));
    expect(within(dialog).queryByTestId(MODES)).toBeNull();
    expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 2 of 3 · Image");
  });

  it("has no item navigation or comparison for a single gallery image", () => {
    renderOverlay([pendingMessage({ images: [before] })]);
    const dialog = openReview();
    expect(within(dialog).queryByTestId(POSITION)).toBeNull();
    expect(within(dialog).queryByTestId(COMPARE_IMAGES)).toBeNull();
  });
});

describe("choice images on the stage", () => {
  it("names the owning choice when two choice images share a description", () => {
    renderOverlay([
      pendingMessage({
        options: [
          {
            option_id: "o1",
            label: "Old",
            description: "",
            images: [{ id: "old", path: "old.png", alt: "Preview" }],
          },
          {
            option_id: "o2",
            label: "New",
            description: "",
            images: [{ id: "new", path: "new.png", alt: "Preview" }],
          },
        ],
      }),
    ]);
    const dialog = openReview();
    expect(within(dialog).getByText(/Preview \(choice: Old\)/)).toBeTruthy();
    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    expect(within(dialog).getByText(/Preview \(choice: New\)/)).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.10
  it("shows a choice's image when the choice is selected, by click or number key", () => {
    renderOverlay([
      pendingMessage({
        images: [billing],
        options: [
          {
            option_id: "o1",
            label: "Old",
            description: "",
            images: [{ id: "old", path: "old.png", alt: "Old header" }],
          },
          { option_id: "o2", label: "Plain", description: "" },
          {
            option_id: "o3",
            label: "New",
            description: "",
            images: [{ id: "new", path: "new.png", alt: "New header" }],
          },
        ],
      }),
    ]);
    const dialog = openReview();
    const position = () => within(dialog).getByTestId(POSITION).textContent;
    expect(position()).toBe("Item 1 of 3 · Image");
    expect(within(dialog).queryByRole("button", { name: /^Show image/ })).toBeNull();
    const [old, plain, fresh] = within(dialog).getAllByTestId("clarification-option");
    expect(within(old).getByTestId(CHOICE_IMAGE).textContent).toContain("Image 2");
    expect(within(plain).queryByTestId(CHOICE_IMAGE)).toBeNull();
    fireEvent.click(fresh);
    expect(position()).toBe("Item 3 of 3 · Image");
    fireEvent.click(plain);
    expect(position()).toBe("Item 3 of 3 · Image");
    fireEvent.keyDown(within(dialog).getByTestId("clarification-question-card"), { key: "1" });
    expect(position()).toBe("Item 2 of 3 · Image");
    fireEvent.click(within(dialog).getByTestId(NEXT_ITEM));
    fireEvent.click(old);
    expect(position()).toBe("Item 2 of 3 · Image");
  });
});

const labelledPair = () =>
  pendingMessage({
    images: [
      { ...before, label: "before" },
      { ...after, label: "after" },
    ],
    image_groups: [{ kind: "pair", image_ids: ["before", "after"] }],
  });

function chooseMode(dialog: HTMLElement, name: string) {
  fireEvent.click(within(within(dialog).getByTestId(MODES)).getByText(name));
}

function identity(dialog: HTMLElement) {
  return within(dialog)
    .getAllByTestId(IDENTITY_BADGE)
    .map((badge) => badge.textContent);
}

describe("visible image identity", () => {
  function wipeLabels(dialog: HTMLElement) {
    return within(dialog)
      .getAllByTestId("question-review-wipe-label")
      .map((label) => `${label.dataset.slot}:${label.textContent}`);
  }

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.2
  it("always names the visible image or sides through flips and modes", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    expect(identity(dialog)).toEqual(["A · before"]);
    fireEvent.click(within(dialog).getByTestId(FLIP));
    expect(identity(dialog)).toEqual(["B · after"]);
    // A pair's order is fixed, so S does not swap it.
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "s" });
    expect(identity(dialog)).toEqual(["B · after"]);

    chooseMode(dialog, "Wipe");
    expect(within(dialog).queryByTestId(IDENTITY_BADGE)).toBeNull();
    expect(wipeLabels(dialog)).toEqual(["a:before", "b:after"]);
    chooseMode(dialog, "Side by side");
    expect(identity(dialog)).toEqual(["A · before", "B · after"]);
    chooseMode(dialog, ONION_SKIN);
    expect(identity(dialog)).toEqual(["TopB · after50%BelowA · before"]);
    chooseMode(dialog, "Difference");
    expect(identity(dialog)).toEqual(["A · before vs B · after"]);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  it("opens the A and B picker lists inside the review dialog", async () => {
    renderOverlay([pendingMessage({ images: [before, after, billing] })]);
    const dialog = openReview();
    fireEvent.click(within(dialog).getByTestId(COMPARE_IMAGES));
    await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
    fireEvent.click(within(dialog).getByTestId("question-review-pick-b"));
    const options = await screen.findAllByTestId("question-review-pick-b-option");
    expect(dialog.contains(options[0])).toBe(true);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.2
  it("renames the sides when an operator comparison swaps its images", async () => {
    renderOverlay([pendingMessage({ images: [before, after, billing] })]);
    const dialog = openReview();
    fireEvent.click(within(dialog).getByTestId(COMPARE_IMAGES));
    await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
    expect(identity(dialog)).toEqual(["A · 1"]);
    fireEvent.click(within(dialog).getByTestId("question-review-swap"));
    expect(identity(dialog)).toEqual(["A · 2"]);
    chooseMode(dialog, "Wipe");
    expect(wipeLabels(dialog)).toEqual(["a:2", "b:1"]);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.3
  it("announces the identity to screen readers in every mode", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    const announcement = within(dialog).getByTestId("question-review-announcement");
    expect(announcement.textContent).toBe("Showing A · before, Before");
    chooseMode(dialog, "Wipe");
    expect(announcement.textContent).toBe(
      "Wipe: A · before, Before on the left; B · after, After on the right",
    );
    for (const label of within(dialog).getAllByTestId("question-review-wipe-label")) {
      expect(label.closest("[aria-hidden='true']")).toBeTruthy();
    }
    chooseMode(dialog, ONION_SKIN);
    expect(announcement.textContent).toBe(
      "Onion skin: B · after, After over A · before, Before at 50%",
    );
    for (const badge of within(dialog).getAllByTestId(IDENTITY_BADGE)) {
      expect(badge.closest("[aria-hidden='true']")).toBeTruthy();
    }
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.3
  it("announces slot names, so images with the same description stay distinct", async () => {
    renderOverlay([
      pendingMessage({
        images: [
          { id: "old", path: "old.png", alt: "Settings screen", label: "old" },
          { id: "new", path: "new.png", alt: "Settings screen", label: "new" },
        ],
        image_groups: [{ kind: "pair", image_ids: ["old", "new"] }],
      }),
    ]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    const announcement = within(dialog).getByTestId("question-review-announcement");
    expect(announcement.textContent).toBe("Showing A · old, Settings screen");
    fireEvent.click(within(dialog).getByTestId(FLIP));
    expect(announcement.textContent).toBe("Showing B · new, Settings screen");
  });

  it("shows no identity badge on a single image", async () => {
    renderOverlay([pendingMessage({ images: [before] })]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    expect(within(dialog).queryByTestId(IDENTITY_BADGE)).toBeNull();
  });
});

describe("Wipe handle and pane controls", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("puts the Wipe names on the handle and shows them while the divider is in use", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    chooseMode(dialog, "Wipe");
    const handle = within(dialog).getByTestId("question-review-wipe-handle");
    expect(within(handle).getAllByTestId("question-review-wipe-label")).toHaveLength(2);
    expect(handle.getAttribute("data-active")).toBe("false");
    fireEvent.pointerEnter(within(dialog).getByTestId("question-review-wipe-slider"));
    expect(handle.getAttribute("data-active")).toBe("true");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  it("does not keep the Wipe names up after leaving Wipe from the divider slider", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    chooseMode(dialog, "Wipe");
    fireEvent.pointerEnter(within(dialog).getByTestId("question-review-wipe-slider"));
    chooseMode(dialog, ONION_SKIN);
    chooseMode(dialog, "Wipe");
    const handle = within(dialog).getByTestId("question-review-wipe-handle");
    expect(handle.getAttribute("data-active")).toBe("false");
  });

  it("leaves Space to a focused control on the image instead of holding to compare", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    fireEvent.click(within(dialog).getByTestId(FLIP));
    const zoomIn = within(dialog).getByTestId("question-review-zoom-in");
    zoomIn.focus();
    fireEvent.keyDown(zoomIn, { key: " " });
    expect(identity(dialog)).toEqual(["B · after"]);
    fireEvent.keyUp(zoomIn, { key: " " });
  });

  it("names each comparison slider for screen readers", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    chooseMode(dialog, "Wipe");
    const slider = within(within(dialog).getByTestId("question-review-wipe-slider")).getByRole(
      "slider",
    );
    expect(slider.getAttribute("aria-label")).toBe("Wipe divider");
    expect(slider.getAttribute("aria-valuetext")).toBe("50%");
    chooseMode(dialog, ONION_SKIN);
    const opacity = within(within(dialog).getByTestId("question-review-opacity")).getByRole(
      "slider",
    );
    expect(opacity.getAttribute("aria-label")).toBe("Opacity of B");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  it("keeps zoom at the far corner in Side by side", async () => {
    renderOverlay([labelledPair()]);
    const dialog = openReview();
    await screen.findAllByTestId(IMAGE);
    chooseMode(dialog, "Side by side");
    const second = within(dialog).getByTestId("question-review-stage-b");
    expect(within(second).queryByTestId("question-review-zoom")).not.toBeNull();
    expect(within(dialog).getAllByTestId("question-review-zoom")).toHaveLength(1);
  });
});

describe("region selection across items", () => {
  function setNaturalSize(image: HTMLElement) {
    Object.defineProperty(image, "naturalWidth", { configurable: true, value: 160 });
    Object.defineProperty(image, "naturalHeight", { configurable: true, value: 100 });
    fireEvent.load(image);
  }

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.5
  it("does not let an earlier region selection override Back to gallery", async () => {
    renderOverlay([pendingMessage({ images: [before, after, billing] })]);
    const dialog = openReview();
    fireEvent.click(within(dialog).getByTestId(COMPARE_IMAGES));
    await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
    within(dialog).getAllByTestId(IMAGE).forEach(setNaturalSize);
    fireEvent.click(within(dialog).getByTestId(FLIP));
    fireEvent.click(within(dialog).getByTestId("question-review-annotate"));
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "n" });
    fireEvent.change(within(dialog).getByTestId("question-review-region-input"), {
      target: { value: "Too tight" },
    });
    fireEvent.click(within(dialog).getByTestId("question-review-region-add"));
    fireEvent.click(within(dialog).getByTestId("question-review-annotate-done"));
    fireEvent.click(within(dialog).getByTestId("question-review-region-edit"));
    fireEvent.keyDown(within(dialog).getByTestId("question-review-region-input"), {
      key: "Escape",
    });

    fireEvent.click(within(dialog).getByTestId("question-review-back-to-gallery"));
    await waitFor(() =>
      expect(within(dialog).getByTestId(POSITION).textContent).toBe("Item 1 of 3 · Image"),
    );
    // A later comparison starts from the current image, not the old region's image.
    fireEvent.click(within(dialog).getByTestId(COMPARE_IMAGES));
    expect(
      within(within(dialog).getByTestId("question-review-announcement")).queryByText(/After/),
    ).toBeNull();
  });
});
