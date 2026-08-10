import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import {
  DedicatedDockerDialog,
  ExternalGoCacheDialog,
  PermanentDeleteDialog,
  QuarantinePurgeDialog,
  TemporaryArtifactsDialog,
} from "./storage-confirmation-dialogs";

afterEach(cleanup);

describe("storage confirmation dialogs", () => {
  // The confirm gate is `confirmation !== phrase`, so the phrase must reach the
  // user exactly as the comparison expects it. Translating it — or letting the
  // <Trans> tag index drift off its <strong> child — would leave a dialog the
  // user cannot satisfy, and nothing else in the suite would fail.
  it("shows the untranslated phrase and only enables the action on an exact match", () => {
    const onConfirm = vi.fn();
    render(
      <PermanentDeleteDialog entry={null} open onOpenChange={vi.fn()} onConfirm={onConfirm} />,
    );

    const description = screen.getByText(/This cannot be undone/);
    expect(description.textContent).toBe(
      "This cannot be undone. Kandev will permanently remove the selected quarantine entry. Type DELETE to continue.",
    );

    const input = screen.getByLabelText("Type DELETE to confirm");
    const action = screen.getByTestId("storage-quarantine-delete-confirm") as HTMLButtonElement;
    expect(action.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "delete" } });
    expect(action.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "DELETE" } });
    expect(action.disabled).toBe(false);
  });

  // Two independent counts cannot share one i18next `count`, so this sentence is
  // two plural messages joined. Both forms are asserted whole.
  it("agrees both quarantine counts with their own number", () => {
    const purge = (eligibleCount: number, protectedCount: number) =>
      render(
        <QuarantinePurgeDialog
          scope="eligible"
          eligibleCount={eligibleCount}
          protectedCount={protectedCount}
          open
          onOpenChange={vi.fn()}
          onConfirm={vi.fn()}
        />,
      );

    purge(1, 1);
    expect(screen.getByText(/This will permanently remove/).textContent).toBe(
      "This will permanently remove 1 eligible item. 1 protected item remains until its retention deadline. Type DELETE ELIGIBLE to continue.",
    );

    cleanup();
    purge(3, 2);
    expect(screen.getByText(/This will permanently remove/).textContent).toBe(
      "This will permanently remove 3 eligible items. 2 protected items remain until their retention deadlines. Type DELETE ELIGIBLE to continue.",
    );
  });

  it("confirms temporary artifact cleanup without a destructive type gate", () => {
    const onConfirm = vi.fn();
    render(<TemporaryArtifactsDialog open onOpenChange={vi.fn()} onConfirm={onConfirm} />);

    expect(screen.getByText(/Eligible registered artifacts/)).toBeTruthy();
    const action = screen.getByTestId("storage-temporary-artifacts-confirm") as HTMLButtonElement;
    expect(action.disabled).toBe(false);
    fireEvent.click(action);
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});

/**
 * The pseudo-locale oracle, at unit scale.
 *
 * `i18next/no-literal-string` runs in `mode: "jsx-only"`, so a string this file
 * hands to `<ConfirmationDialog>` through a helper or a template would be
 * invisible to lint, and a clean lint is not proof the file is done. Under
 * `pseudo` every catalog message is accented, so any word-like ASCII left in a
 * dialog is a literal that never reached the catalog — except the confirmation
 * phrase, which must stay verbatim for `confirmation !== phrase` to be
 * satisfiable. These four dialogs carry the page's destructive actions, so they
 * are the ones worth pinning.
 *
 * Accessibility copy is scanned too. A screen-reader-only string is exactly the
 * kind that survives a migration untranslated, because nothing on screen looks
 * wrong — and the input's `aria-label` is the only place the confirmation phrase
 * is announced.
 */
describe("storage confirmation dialogs under the pseudo-locale", () => {
  const ACCENTED = /[À-ɏ]/;
  const WORDLIKE = /[A-Za-z]{4,}/;
  /** Attributes that carry copy, mirroring the eslint guard's include list. */
  const COPY_ATTRIBUTES = ["aria-label", "aria-description", "title", "placeholder", "alt"];

  /** Text and accessibility copy in the open dialog that is not accented. */
  function unlocalizedText(): string[] {
    const dialog = document.querySelector("[role=alertdialog]");
    if (!dialog) throw new Error("dialog did not render");
    const leftovers = new Set<string>();
    const consider = (value: string | null | undefined) => {
      const text = (value ?? "").trim();
      if (text && WORDLIKE.test(text) && !ACCENTED.test(text)) leftovers.add(text);
    };

    const walker = document.createTreeWalker(dialog, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      consider(node.textContent);
      node = walker.nextNode();
    }
    for (const element of [dialog, ...dialog.querySelectorAll("*")]) {
      for (const attribute of COPY_ATTRIBUTES) consider(element.getAttribute(attribute));
    }
    return [...leftovers];
  }

  beforeAll(async () => {
    await activateLocale("pseudo");
  });
  afterAll(async () => {
    await activateLocale("en");
  });

  const dialogProps = { open: true, onOpenChange: vi.fn(), onConfirm: vi.fn() };

  it.each([
    ["DEDICATED", <DedicatedDockerDialog key="d" {...dialogProps} />],
    ["ADOPT", <ExternalGoCacheDialog key="a" path="" {...dialogProps} />],
    ["DELETE", <PermanentDeleteDialog key="p" entry={null} {...dialogProps} />],
    [
      "DELETE ALL NOW",
      <QuarantinePurgeDialog
        key="q"
        scope="all"
        eligibleCount={0}
        protectedCount={0}
        {...dialogProps}
      />,
    ],
  ])("leaves only the %s phrase un-accented", (phrase, element) => {
    render(element);
    // The phrase appears twice — in the sentence and in the input's aria-label —
    // and both must be the raw token, so the deduped set is exactly one entry.
    expect(unlocalizedText()).toEqual([phrase]);
    const input = screen.getByRole("textbox");
    const label = input.getAttribute("aria-label") ?? "";
    expect(label).toContain(phrase);
    expect(label.replace(phrase, "")).toMatch(ACCENTED);
  });
});
