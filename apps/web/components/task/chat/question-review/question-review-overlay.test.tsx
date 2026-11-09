import { createRef } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

/* eslint-disable max-lines -- review view regressions share one overlay harness. */

const mocks = vi.hoisted(() => ({ fetchImage: vi.fn(), updateMessage: vi.fn() }));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));
vi.mock("@/lib/api/domains/question-image-api", () => ({
  fetchQuestionImage: mocks.fetchImage,
  questionImageErrorCode: () => "unavailable",
}));
const mockStore = {
  getState: () => ({ updateMessage: mocks.updateMessage }),
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
const REGION_INPUT = "question-review-region-input";
const REGION_POPOVER = "question-review-region-popover";
const REGION_ITEM = "question-review-region-item";
const REGION_TEXT = "question-review-region-text";
const ZOOM_LEVEL = "question-review-zoom-level";
const SUBMIT = "question-review-submit";
const IMAGE = "question-review-image";
const ANNOTATE = "question-review-annotate";
const REVIEW_DIALOG = "question-review-dialog";
const QUESTION_CARD = "clarification-question-card";
const SEGMENT = "question-review-segment";
const OPTION = "clarification-option";
const DATA_SELECTED = "data-selected";
const STAGE = "question-review-stage";
const CLOSE = "question-review-close";
const DATA_QUESTION_ID = "data-question-id";
const OPEN = "question-review-open";
const INPUT = "clarification-input";
const before = { id: "before", path: "before.png", alt: "Before" };
const after = { id: "after", path: "after.png", alt: "After" };
const spacing = { id: "spacing", path: "spacing.png", alt: "Spacing" };

function message(index: number, total: number, question: Partial<ClarificationQuestion>): Message {
  const questionId = `q${index + 1}`;
  return {
    id: `m${index + 1}`,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-11-07T00:00:00Z",
    metadata: {
      pending_id: "p1",
      question_id: questionId,
      question_index: index,
      question_total: total,
      status: "pending",
      question: {
        id: questionId,
        title: "",
        prompt: `Question ${index + 1}?`,
        options: [
          { option_id: `${questionId}_o1`, label: "Approve", description: "" },
          { option_id: `${questionId}_o2`, label: "Revise", description: "" },
        ],
        ...question,
      },
    },
  };
}

function renderOverlay(messages: Message[]) {
  const scopeRef = createRef<HTMLDivElement>();
  const onDismiss = vi.fn();
  const utils = render(
    <ClarificationDraftProvider taskId="t1">
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={messages}
          onResolved={vi.fn()}
          onDismiss={onDismiss}
          shortcutScopeRef={scopeRef}
        />
      </div>
    </ClarificationDraftProvider>,
  );
  return { ...utils, onDismiss, scopeRef };
}

const imageBundle = () => [
  message(0, 2, { images: [before, after] }),
  message(1, 2, { images: [spacing] }),
];

function openReview() {
  fireEvent.click(screen.getByTestId(OPEN));
  return screen.getByTestId(REVIEW_DIALOG);
}

function chooseZoom(dialog: HTMLElement, testId: string) {
  fireEvent.click(within(dialog).getByTestId(ZOOM_LEVEL));
  fireEvent.click(screen.getByTestId(testId));
}

function respondCalls() {
  return fetchMock.mock.calls.filter(([url]) => String(url).includes("/respond"));
}

beforeEach(() => {
  fetchMock.mockReset();
  mocks.fetchImage.mockReset();
  mocks.fetchImage.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
  fetchMock.mockResolvedValue(new Response(JSON.stringify({ success: true }), { status: 200 }));
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
  vi.stubGlobal(
    "URL",
    Object.assign(URL, {
      createObjectURL: vi.fn(() => "blob:test"),
      revokeObjectURL: vi.fn(),
    }),
  );
});

afterEach(() => {
  cleanup();
  // Retained answer drafts are keyed by the store; reset them so tests stay independent.
  clearAnswerDrafts(mockStore);
});

describe("question image review control", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  it("is offered for image bundles and not for image-free bundles", () => {
    const { unmount } = renderOverlay(imageBundle());
    expect(screen.getByTestId(OPEN)).toBeTruthy();
    unmount();
    renderOverlay([message(0, 1, {})]);
    expect(screen.queryByTestId(OPEN)).toBeNull();
  });

  it("is offered when only a choice carries an image", () => {
    renderOverlay([
      message(0, 1, {
        options: [
          { option_id: "o1", label: "Wide", description: "", images: [before] },
          { option_id: "o2", label: "Narrow", description: "" },
        ],
      }),
    ]);
    expect(screen.getByTestId(OPEN)).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.2
  it("keeps the compact panel as the default", () => {
    renderOverlay(imageBundle());
    expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull();
    expect(screen.getByTestId(QUESTION_CARD)).toBeTruthy();
  });
});

describe("question image review view", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-002.1
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.4
  it("moves the answer form below the stage without submitting", async () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    expect(within(dialog).getByTestId(STAGE)).toBeTruthy();
    expect(within(dialog).getByTestId(QUESTION_CARD)).toBeTruthy();
    expect(screen.getByTestId("question-review-placeholder")).toBeTruthy();
    expect(screen.getAllByTestId(QUESTION_CARD)).toHaveLength(1);
    expect(within(dialog).queryByTestId("question-images")).toBeNull();
    fireEvent.click(within(dialog).getByTestId(CLOSE));
    await waitFor(() => expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull());
    expect(respondCalls()).toHaveLength(0);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.3
  it("shares selections and text drafts with the compact panel in both directions", async () => {
    renderOverlay(imageBundle());
    fireEvent.click(screen.getAllByTestId(OPTION)[1]);
    let dialog = openReview();
    const step = within(dialog).getByTestId(QUESTION_CARD);
    expect(step.getAttribute(DATA_QUESTION_ID)).toBe("q2");
    fireEvent.click(within(dialog).getAllByTestId(SEGMENT)[0]);
    expect(within(dialog).getAllByTestId(OPTION)[1].getAttribute(DATA_SELECTED)).toBe("true");
    fireEvent.change(within(dialog).getByTestId(INPUT), {
      target: { value: "Tighten the header" },
    });
    fireEvent.click(within(dialog).getByTestId(CLOSE));
    await waitFor(() => expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull());
    expect((screen.getByTestId(INPUT) as HTMLTextAreaElement).value).toBe("Tighten the header");
    dialog = openReview();
    expect((within(dialog).getByTestId(INPUT) as HTMLTextAreaElement).value).toBe(
      "Tighten the header",
    );
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.5
  it("closes on Escape without dismissing the question and returns focus", async () => {
    const { onDismiss } = renderOverlay(imageBundle());
    const trigger = screen.getByTestId(OPEN);
    const dialog = openReview();
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "Escape" });
    await waitFor(() => expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull());
    expect(onDismiss).not.toHaveBeenCalled();
    expect(respondCalls()).toHaveLength(0);
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("question image review keyboard and stage", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-002.4
  it("keeps stage keys away from option, navigation, and submit shortcuts", () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    const stage = within(dialog).getByTestId(STAGE);
    for (const key of ["1", "2", "ArrowRight", "ArrowLeft"]) fireEvent.keyDown(stage, { key });
    fireEvent.keyDown(stage, { key: "Enter", metaKey: true });
    const card = within(dialog).getByTestId(QUESTION_CARD);
    expect(card.getAttribute(DATA_QUESTION_ID)).toBe("q1");
    expect(
      within(dialog)
        .getAllByTestId(OPTION)
        .some((option) => option.getAttribute(DATA_SELECTED) === "true"),
    ).toBe(false);
    expect(respondCalls()).toHaveLength(0);
  });

  it("keeps form shortcuts working when focus is in the form", () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    const card = within(dialog).getByTestId(QUESTION_CARD);
    fireEvent.keyDown(card, { key: "2" });
    expect(within(dialog).getAllByTestId(OPTION)[1].getAttribute(DATA_SELECTED)).toBe("true");
    fireEvent.keyDown(card, { key: "ArrowRight" });
    expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q2");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  it("zooms by toolbar and keyboard", () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    const level = within(dialog).getByTestId(ZOOM_LEVEL);
    chooseZoom(dialog, "question-review-actual");
    expect(level.textContent).toBe("100%");
    fireEvent.click(within(dialog).getByTestId("question-review-zoom-in"));
    expect(level.textContent).toBe("200%");
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "-" });
    expect(level.textContent).toBe("100%");
  });
});

describe("question image review segments and submission", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-003.1
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-003.3
  it("navigates segments and fetches only the active segment's images", async () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    await waitFor(() => expect(mocks.fetchImage).toHaveBeenCalledTimes(2));
    expect(mocks.fetchImage.mock.calls.map((call) => call[2])).toEqual(["before", "after"]);
    const segments = within(dialog).getAllByTestId(SEGMENT);
    expect(segments[0].getAttribute("aria-current")).toBe("step");
    fireEvent.click(segments[1]);
    await waitFor(() => expect(mocks.fetchImage).toHaveBeenCalledTimes(3));
    expect(mocks.fetchImage.mock.calls[2].slice(0, 3)).toEqual(["p1", "q2", "spacing"]);
    expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q2");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.4
  it("submits through the existing path and then closes", async () => {
    renderOverlay([message(0, 1, { images: [before], allow_custom_text: false })]);
    const dialog = openReview();
    fireEvent.click(within(dialog).getAllByTestId(OPTION)[0]);
    await act(async () => {
      fireEvent.click(within(dialog).getByTestId(SUBMIT));
    });
    await waitFor(() => expect(respondCalls()).toHaveLength(1));
    await waitFor(() => expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull());
  });

  it("shows a choice's stage image number instead of its thumbnail", () => {
    renderOverlay([
      message(0, 1, {
        images: [before],
        options: [
          { option_id: "o1", label: "Wide", description: "", images: [after] },
          { option_id: "o2", label: "Narrow", description: "" },
        ],
      }),
    ]);
    const dialog = openReview();
    expect(within(dialog).getByTestId("question-review-choice-image").textContent).toBe(
      "Image 2 on the stage",
    );
  });
});

function setNaturalSize(image: HTMLElement, width: number, height: number) {
  Object.defineProperty(image, "naturalWidth", { configurable: true, value: width });
  Object.defineProperty(image, "naturalHeight", { configurable: true, value: height });
  fireEvent.load(image);
}

async function openCompare() {
  renderOverlay(imageBundle());
  const dialog = openReview();
  await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
  return dialog;
}

function pickMode(dialog: HTMLElement, name: string) {
  fireEvent.click(within(within(dialog).getByTestId("question-review-modes")).getByText(name));
}

describe("question image review comparison", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.1
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.2
  it("starts in Toggle on A and flips by control and stage key", async () => {
    const dialog = await openCompare();
    const announcement = within(dialog).getByTestId("question-review-announcement");
    expect(announcement.textContent).toBe("Showing A: Before");
    fireEvent.click(within(dialog).getByTestId("question-review-flip"));
    expect(announcement.textContent).toBe("Showing B: After");
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "t" });
    expect(announcement.textContent).toBe("Showing A: Before");
    const [before, after] = within(dialog).getAllByTestId(IMAGE);
    expect(after.className).toContain("invisible");
    expect(before.className).not.toContain("invisible");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.7
  it("keeps zoom when switching among Toggle, Wipe, Side by side, and Onion skin", async () => {
    const dialog = await openCompare();
    const level = within(dialog).getByTestId(ZOOM_LEVEL);
    chooseZoom(dialog, "question-review-actual");
    fireEvent.click(within(dialog).getByTestId("question-review-zoom-in"));
    expect(level.textContent).toBe("200%");
    for (const mode of ["Wipe", "Side by side", "Onion skin", "Toggle"]) {
      pickMode(dialog, mode);
      expect(within(dialog).getByTestId(ZOOM_LEVEL).textContent).toBe("200%");
    }
    fireEvent.click(within(dialog).getByTestId("question-review-swap"));
    expect(within(dialog).getByTestId(ZOOM_LEVEL).textContent).toBe("200%");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.3
  it("moves the wipe divider by keyboard without panning or changing the question", async () => {
    const dialog = await openCompare();
    pickMode(dialog, "Wipe");
    const handle = within(dialog).getByTestId("question-review-wipe-handle");
    expect(handle.getAttribute("aria-valuenow")).toBe("50");
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(handle.getAttribute("aria-valuenow")).toBe("55");
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "]" });
    expect(handle.getAttribute("aria-valuenow")).toBe("60");
    expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q1");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.4
  it("shows Side by side as two panes", async () => {
    const dialog = await openCompare();
    pickMode(dialog, "Side by side");
    expect(within(dialog).getByTestId(STAGE)).toBeTruthy();
    expect(within(dialog).getByTestId("question-review-stage-b")).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.5
  it("applies the onion-skin opacity to B", async () => {
    const dialog = await openCompare();
    pickMode(dialog, "Onion skin");
    const after = within(dialog).getAllByTestId(IMAGE)[1];
    expect(after.style.opacity).toBe("0.5");
    expect(within(dialog).getByTestId("question-review-opacity")).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.9
  it("shows Difference as unavailable when the browser cannot compute it", async () => {
    const dialog = await openCompare();
    pickMode(dialog, "Difference");
    await waitFor(() =>
      expect(within(dialog).getByTestId("question-review-diff-status").textContent).toBe(
        "Difference is not available for these images on this device.",
      ),
    );
    expect(within(dialog).getByTestId(QUESTION_CARD)).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.8
  it("reports different image sizes", async () => {
    const dialog = await openCompare();
    const [before, after] = within(dialog).getAllByTestId(IMAGE);
    setNaturalSize(before, 160, 100);
    setNaturalSize(after, 160, 104);
    expect(within(dialog).getByTestId("question-review-sizes-differ").textContent).toBe(
      "Sizes differ: A 160 × 100 px, B 160 × 104 px",
    );
  });
});

async function openAnnotatable() {
  renderOverlay([message(0, 1, { images: [before, after], allow_custom_text: false })]);
  const dialog = openReview();
  await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
  for (const image of within(dialog).getAllByTestId(IMAGE)) {
    setNaturalSize(image, 160, 100);
  }
  return dialog;
}

function respondBody() {
  const [, init] = respondCalls()[0];
  return JSON.parse(String((init as RequestInit).body));
}

function drawRegionByKeyboard(dialog: HTMLElement) {
  fireEvent.click(within(dialog).getByTestId(ANNOTATE));
  fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "n" });
}

function commentRegion(dialog: HTMLElement, text: string) {
  fireEvent.change(within(dialog).getByTestId(REGION_INPUT), {
    target: { value: text },
  });
  fireEvent.click(within(dialog).getByTestId("question-review-region-add"));
}

describe("question image review regions", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.7
  it("opens a comment popover for a new region and moves, edits, and deletes it", async () => {
    const dialog = await openAnnotatable();
    drawRegionByKeyboard(dialog);
    const input = within(dialog).getByTestId(REGION_INPUT);
    expect(document.activeElement).toBe(input);
    commentRegion(dialog, "Spacing is off");
    expect(within(dialog).queryByTestId(REGION_POPOVER)).toBeNull();
    const items = within(dialog).getAllByTestId(REGION_ITEM);
    expect(items).toHaveLength(1);
    expect(items[0].textContent).toContain("Image 1: Before");
    expect(within(dialog).getByTestId(REGION_TEXT).textContent).toBe("Spacing is off");
    const stage = within(dialog).getByTestId(STAGE);
    const left = within(dialog).getByTestId("question-review-region").style.left;
    fireEvent.keyDown(stage, { key: "ArrowRight", shiftKey: true });
    expect(within(dialog).getByTestId("question-review-region").style.left).not.toBe(left);
    fireEvent.click(within(dialog).getByTestId("question-review-region-edit"));
    const editInput = within(dialog).getByTestId(REGION_INPUT);
    expect((editInput as HTMLTextAreaElement).value).toBe("Spacing is off");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(within(dialog).getAllByTestId(REGION_ITEM)).toHaveLength(1);
    fireEvent.keyDown(stage, { key: "Delete" });
    expect(within(dialog).queryAllByTestId(REGION_ITEM)).toHaveLength(0);
  });

  it("discards a new region when its comment is cancelled, without closing the view", async () => {
    const dialog = await openAnnotatable();
    drawRegionByKeyboard(dialog);
    expect(within(dialog).getByTestId(REGION_POPOVER)).toBeTruthy();
    fireEvent.keyDown(within(dialog).getByTestId(REGION_INPUT), {
      key: "Escape",
    });
    await waitFor(() => expect(within(dialog).queryByTestId(REGION_POPOVER)).toBeNull());
    expect(within(dialog).queryAllByTestId(REGION_ITEM)).toHaveLength(0);
    expect(screen.getByTestId(REVIEW_DIALOG)).toBeTruthy();
  });

  it("allows annotating only in the single-image view and Toggle", async () => {
    const dialog = await openAnnotatable();
    pickMode(dialog, "Wipe");
    expect(within(dialog).getByTestId(ANNOTATE)).toHaveProperty("disabled", true);
    expect(within(dialog).getByTestId("question-review-wipe-slider")).toBeTruthy();
    pickMode(dialog, "Toggle");
    expect(within(dialog).getByTestId(ANNOTATE)).toHaveProperty("disabled", false);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.3
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.5
  it("keeps regions across close and submits them with the answer", async () => {
    const dialog = await openAnnotatable();
    fireEvent.click(within(dialog).getByTestId("question-review-flip"));
    drawRegionByKeyboard(dialog);
    commentRegion(dialog, "Label contrast too low");
    fireEvent.click(within(dialog).getAllByTestId(OPTION)[1]);
    fireEvent.click(within(dialog).getByTestId(CLOSE));
    await waitFor(() => expect(screen.queryByTestId(REVIEW_DIALOG)).toBeNull());
    const reopened = openReview();
    expect(within(reopened).getByTestId(REGION_TEXT).textContent).toBe("Label contrast too low");
    await act(async () => {
      fireEvent.click(within(reopened).getByTestId(SUBMIT));
    });
    await waitFor(() => expect(respondCalls()).toHaveLength(1));
    const [answer] = respondBody().answers;
    expect(answer.selected_options).toEqual(["q1_o2"]);
    expect(answer.annotations).toEqual([
      expect.objectContaining({
        image_id: "after",
        image_width: 160,
        image_height: 100,
        comment: "Label contrast too low",
      }),
    ]);
  });
});

describe("answering in the review view", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.3
  it("records a choice without moving on, and emphasizes Next", () => {
    renderOverlay(imageBundle());
    const dialog = openReview();
    fireEvent.click(within(dialog).getAllByTestId(OPTION)[0]);
    expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q1");
    expect(within(dialog).getAllByTestId(OPTION)[0].getAttribute(DATA_SELECTED)).toBe("true");
    const next = within(dialog).getByTestId("clarification-next");
    expect(next.getAttribute("data-emphasized")).toBe("true");
    fireEvent.click(next);
    expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q2");
  });

  it("still moves on after a choice in the compact panel", () => {
    renderOverlay(imageBundle());
    fireEvent.click(screen.getAllByTestId(OPTION)[0]);
    expect(screen.getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q2");
  });
});

const labelled = [
  { ...before, label: "before" },
  { ...after, label: "after" },
  { id: "alt", path: "alt.png", alt: "Alternative", label: "alt" },
];

async function openThree() {
  renderOverlay([
    message(0, 2, { images: labelled }),
    message(1, 2, { images: [spacing, { id: "spacing2", path: "s2.png", alt: "Spacing 2" }] }),
  ]);
  const dialog = openReview();
  await waitFor(() => expect(within(dialog).getAllByTestId(IMAGE)).toHaveLength(2));
  return dialog;
}

describe("review usability round", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-009.1
  it("shows agent labels in the pickers", async () => {
    const dialog = await openThree();
    expect(
      within(within(dialog).getByTestId("question-review-pick-a")).getByText("before"),
    ).toBeTruthy();
    expect(
      within(within(dialog).getByTestId("question-review-pick-b")).getByText("after"),
    ).toBeTruthy();
    fireEvent.click(within(dialog).getByTestId("question-review-pick-b"));
    const options = screen.getAllByTestId("question-review-pick-b-option");
    expect(options.map((option) => option.firstChild?.textContent)).toEqual([
      "before",
      "after",
      "alt",
    ]);
    fireEvent.click(options[2]);
    expect(
      within(within(dialog).getByTestId("question-review-pick-b")).getByText("alt"),
    ).toBeTruthy();
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-002.5
  it("keeps zoom on the image and offers Zoom to changes outside Difference", async () => {
    const dialog = await openThree();
    const stage = within(dialog).getByTestId(STAGE);
    expect(within(stage).getByTestId("question-review-zoom")).toBeTruthy();
    expect(within(stage).getByTestId("question-review-zoom-changes")).toBeTruthy();
    expect(within(stage).getByTestId(ANNOTATE)).toBeTruthy();
    fireEvent.doubleClick(stage);
    expect(within(dialog).getByTestId(ZOOM_LEVEL).textContent).toBe("100%");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.11
  it("steps B through the candidates by button and key", async () => {
    const dialog = await openThree();
    const label = within(dialog).getByTestId("question-review-step-label");
    expect(label.textContent).toBe("B: after (1 of 2)");
    fireEvent.click(within(dialog).getByTestId("question-review-step-next"));
    expect(label.textContent).toBe("B: alt (2 of 2)");
    expect(within(dialog).getByTestId("question-review-step-next")).toHaveProperty(
      "disabled",
      true,
    );
    fireEvent.keyDown(within(dialog).getByTestId(STAGE), { key: "[" });
    expect(within(dialog).getByTestId("question-review-step-label").textContent).toBe(
      "B: after (1 of 2)",
    );
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-005.12
  it("keeps the comparison mode when moving to another question", async () => {
    const dialog = await openThree();
    pickMode(dialog, "Onion skin");
    fireEvent.click(within(dialog).getAllByTestId(SEGMENT)[1]);
    await waitFor(() =>
      expect(within(dialog).getByTestId(QUESTION_CARD).getAttribute(DATA_QUESTION_ID)).toBe("q2"),
    );
    expect(within(dialog).getByTestId("question-review-opacity")).toBeTruthy();
  });

  it("shows A while Space is held", async () => {
    const dialog = await openThree();
    fireEvent.click(within(dialog).getByTestId("question-review-flip"));
    const announcement = within(dialog).getByTestId("question-review-announcement");
    expect(announcement.textContent).toBe("Showing B: After");
    const stage = within(dialog).getByTestId(STAGE);
    fireEvent.keyDown(stage, { key: " " });
    expect(announcement.textContent).toBe("Showing A: Before");
    fireEvent.keyUp(stage, { key: " " });
    expect(announcement.textContent).toBe("Showing B: After");
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  it("makes marking obvious and leaves it on Escape before closing the view", async () => {
    const dialog = await openAnnotatable();
    const button = within(dialog).getByTestId(ANNOTATE);
    expect(button.textContent).toBe("Mark area");
    fireEvent.click(button);
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.textContent).toBe("Marking areas");
    expect(within(dialog).getByTestId("question-review-annotate-hint")).toBeTruthy();
    expect(within(dialog).getByTestId("question-review-regions-panel")).toBeTruthy();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(button.getAttribute("aria-pressed")).toBe("false"));
    expect(screen.getByTestId(REVIEW_DIALOG)).toBeTruthy();
  });

  it("opens a region for editing when it is clicked", async () => {
    const dialog = await openAnnotatable();
    drawRegionByKeyboard(dialog);
    commentRegion(dialog, "Too tight");
    fireEvent.click(within(dialog).getByTestId("question-review-annotate-done"));
    const region = within(dialog).getByTestId("question-review-region");
    fireEvent.pointerDown(region, { pointerId: 1, clientX: 10, clientY: 10, isPrimary: true });
    fireEvent.pointerUp(region, { pointerId: 1, clientX: 11, clientY: 10, isPrimary: true });
    expect((within(dialog).getByTestId(REGION_INPUT) as HTMLTextAreaElement).value).toBe(
      "Too tight",
    );
  });
});
