import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCommentsStore, type Comment } from "@/lib/state/slices/comments";
import {
  acknowledge,
  assertRetained,
  clickDelivery,
  closeDelivery,
  commentsFor,
  deliveryButton,
  disconnectDelivery,
  disposeDeliveryFixture,
  flushDelivery,
  isReviewOpen,
  openDelivery,
  prepareDeliveryFixture,
  reconnectDelivery,
  reopenDelivery,
  restoreDeliveryConnection,
  sends,
  submittedMarkdown,
  takeDeliveryOffline,
  type Surface,
} from "./dockview-review-dialog.delivery.test-helpers";

for (const surface of ["desktop", "phone"] as const) {
  describe(`${surface} actual Review delivery`, () => {
    beforeEach(() => prepareDeliveryFixture(surface));
    afterEach(disposeDeliveryFixture);
    registerFailureTests(surface);
    registerOfflineTests(surface);
    registerAcknowledgementTests(surface);
    registerOwnershipTests(surface);
  });
}

function registerFailureTests(surface: Surface) {
  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.1 .5 .6 .8
  it("retains exact file and line notes on rejection and acknowledges a deliberate retry", async () => {
    const { primary, notes } = await openDelivery(surface);
    clickDelivery(surface);
    expect(sends()).toHaveLength(1);
    await acknowledge(0, "server refused this feedback");
    assertRetained(notes, primary.sessionId);
    expect.soft(isReviewOpen()).toBe(true);
    expect(screen.getByText("Failed to send comments")).toBeDefined();
    if (!isReviewOpen()) return;
    expect(deliveryButton().hasAttribute("disabled")).toBe(false);
    clickDelivery(surface);
    expect(sends()).toHaveLength(2);
    expect(sends()[1].payload.content).toBe(submittedMarkdown);
    expect(sends()[1].payload.client_message_id).not.toBe(sends()[0].payload.client_message_id);
    await acknowledge(1);
    assertRetained([], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.2 .6 .8
  it("unavailable transport preserves persisted feedback and allows a later send", async () => {
    const { primary, notes } = await openDelivery(surface);
    disconnectDelivery();
    clickDelivery(surface);
    await flushDelivery();
    expect(sends()).toHaveLength(0);
    assertRetained(notes, primary.sessionId);
    expect.soft(isReviewOpen()).toBe(true);
    expect.soft(screen.queryByText("Failed to send comments")).not.toBeNull();
    if (!isReviewOpen()) return;
    reconnectDelivery();
    clickDelivery(surface);
    expect(sends()).toHaveLength(1);
    await acknowledge();
    assertRetained([], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.3 .8
  it("deferred delivery retains notes and admits only one rapid or reopened send", async () => {
    const { primary, notes } = await openDelivery(surface);
    clickDelivery(surface);
    expect(sends()).toHaveLength(1);
    assertRetained(notes, primary.sessionId);
    expect.soft(isReviewOpen()).toBe(true);
    if (!isReviewOpen()) return;
    expect(deliveryButton().hasAttribute("disabled")).toBe(true);
    fireEvent.click(deliveryButton());
    closeDelivery();
    await reopenDelivery();
    expect(deliveryButton().hasAttribute("disabled")).toBe(true);
    fireEvent.click(deliveryButton());
    expect(sends()).toHaveLength(1);
    await acknowledge();
    assertRetained([], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });
}

function registerOfflineTests(surface: Surface) {
  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.2 .3 .6 .8 .9
  it("a registered disconnected client preserves retry without queuing an offline send", async () => {
    const { primary, notes } = await openDelivery(surface);
    takeDeliveryOffline();
    clickDelivery(surface);
    await flushDelivery();
    expect(sends()).toHaveLength(0);
    assertRetained(notes, primary.sessionId);
    expect(isReviewOpen()).toBe(true);
    expect.soft(deliveryButton().hasAttribute("disabled")).toBe(false);
    expect.soft(screen.queryByText("Failed to send comments")).not.toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10001);
    });
    expect.soft(deliveryButton().hasAttribute("disabled")).toBe(false);
    closeDelivery();
    await reopenDelivery();
    expect.soft(deliveryButton().hasAttribute("disabled")).toBe(false);
    await restoreDeliveryConnection();
    expect.soft(sends()).toHaveLength(0);
    if (sends().length > 0) {
      await acknowledge();
      return;
    }
    assertRetained(notes, primary.sessionId);
    clickDelivery(surface);
    expect(sends()).toHaveLength(1);
    expect(sends()[0].payload.content).toBe(submittedMarkdown);
    await acknowledge();
    assertRetained([], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });
}

function registerAcknowledgementTests(surface: Surface) {
  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.4 .5 .7 .8
  it("acknowledged unchanged feedback clears exactly the submitted IDs and closes", async () => {
    const { primary, other } = await openDelivery(surface);
    const unrelated = commentsFor(other.sessionId);
    act(() => unrelated.forEach((c) => useCommentsStore.getState().addComment(c)));
    clickDelivery(surface);
    expect(sends()).toHaveLength(1);
    expect(sends()[0].payload).toEqual({
      task_id: primary.taskId,
      session_id: primary.sessionId,
      content: submittedMarkdown,
      client_message_id: expect.stringMatching(
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
      ),
    });
    await acknowledge();
    assertRetained([], primary.sessionId);
    assertRetained(unrelated, other.sessionId);
    expect(isReviewOpen()).toBe(false);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.4 .5 .7 .8
  it("acknowledgement preserves edited text, new notes and unrelated sources/sessions", async () => {
    const { primary, other, notes } = await openDelivery(surface);
    clickDelivery(surface);
    const newNote = {
      ...notes[1],
      id: `later-${primary.sessionId}`,
      text: "  New feedback\n  after sending  ",
    };
    const otherNotes = commentsFor(other.sessionId);
    const editorNote: Comment = {
      id: "editor-feedback",
      source: "file-editor",
      status: "pending",
      sessionId: primary.sessionId,
      filePath: "unrelated.ts",
      selectedText: "draft",
      text: "Keep editor note",
      createdAt: "2026-10-08T10:02:00Z",
    };
    const edited = { ...notes[0], text: "\n  New correction, preserve every space.  \n" };
    act(() => {
      useCommentsStore.getState().updateComment(edited.id, { text: edited.text });
      [newNote, editorNote, ...otherNotes].forEach((c) =>
        useCommentsStore.getState().addComment(c),
      );
    });
    expect(sends()[0].payload.content).toBe(submittedMarkdown);
    await acknowledge();
    assertRetained([edited, newNote, editorNote], primary.sessionId);
    assertRetained(otherNotes, other.sessionId);
    expect(isReviewOpen()).toBe(true);
    expect(deliveryButton().hasAttribute("disabled")).toBe(false);
    clickDelivery(surface);
    expect(sends()).toHaveLength(2);
    expect(sends()[1].payload.content).toContain(edited.text);
    expect(sends()[1].payload.content).not.toContain(editorNote.text);
    await acknowledge(1);
    assertRetained([editorNote], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.4 .5 .8
  it("acknowledgement preserves changed repository and anchor metadata under the same ID", async () => {
    const { primary, notes } = await openDelivery(surface);
    clickDelivery(surface);
    const line = {
      ...notes[0],
      startLine: 12,
      endLine: 13,
      codeContent: "new anchor",
      repositoryName: "other-core",
    };
    const file = {
      ...notes[1],
      baseRef: "new-gitlink",
      isSubmodule: false,
      repositoryId: "new-repo",
    };
    act(() => {
      useCommentsStore.getState().updateComment(line.id, line);
      useCommentsStore.getState().updateComment(file.id, file);
    });
    await acknowledge();
    assertRetained([line, file], primary.sessionId);
    expect(sends()[0].payload.content).toBe(submittedMarkdown);
    expect(isReviewOpen()).toBe(true);
  });
}

function registerOwnershipTests(surface: Surface) {
  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.4 .7
  it("deleted submitted feedback stays deleted while other notes survive", async () => {
    const { primary, notes } = await openDelivery(surface);
    clickDelivery(surface);
    const later = { ...notes[1], id: `after-delete-${primary.sessionId}`, text: "Remaining work" };
    act(() => {
      useCommentsStore.getState().removeComment(notes[0].id);
      useCommentsStore.getState().addComment(later);
    });
    await acknowledge();
    assertRetained([later], primary.sessionId);
    expect(useCommentsStore.getState().byId[notes[0].id]).toBeUndefined();
    expect(isReviewOpen()).toBe(true);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.4 .5 .7 .8
  it("a previous session acknowledgement does not close or clear the current Review", async () => {
    const fixture = await openDelivery(surface);
    clickDelivery(surface);
    const next = commentsFor(fixture.other.sessionId);
    act(() => next.forEach((c) => useCommentsStore.getState().addComment(c)));
    await fixture.switchToOther();
    await reopenDelivery();
    await acknowledge();
    assertRetained([], fixture.primary.sessionId);
    assertRetained(next, fixture.other.sessionId);
    expect(isReviewOpen()).toBe(true);
    clickDelivery(surface);
    expect(sends()).toHaveLength(2);
    expect(sends()[1].payload.session_id).toBe(fixture.other.sessionId);
    expect(sends()[1].payload.task_id).toBe(fixture.other.taskId);
    await acknowledge(1);
    assertRetained([], fixture.other.sessionId);
    expect(isReviewOpen()).toBe(false);
  });

  // @covers AC-UI-REVIEW-COMMENT-DELIVERY-001.3 .7 .9
  it("explicit dismissal and rejected uncertain transport never reopen or automatically resend", async () => {
    const { primary, notes } = await openDelivery(surface);
    clickDelivery(surface);
    if (isReviewOpen()) closeDelivery();
    await acknowledge(0, "Connection result unknown");
    assertRetained(notes, primary.sessionId);
    expect(isReviewOpen()).toBe(false);
    expect(sends()).toHaveLength(1);
    await reopenDelivery();
    clickDelivery(surface);
    expect(sends()).toHaveLength(2);
    closeDelivery();
    await acknowledge(1);
    assertRetained([], primary.sessionId);
    expect(isReviewOpen()).toBe(false);
  });
}
