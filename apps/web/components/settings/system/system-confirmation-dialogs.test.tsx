import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { FactoryResetDialog } from "./factory-reset-dialog";
import { RestoreDialog } from "./restore-dialog";

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: () => null,
  useSystemJobs: () => [],
}));

afterEach(cleanup);

/**
 * Both dialogs gate their confirm button on `typed === CONFIRM_TOKEN` and send
 * the same token to the API, and both actions are irreversible. Translating a
 * token would leave a dialog the user cannot satisfy in that locale, and
 * nothing would fail until a second language shipped.
 */
describe("system type-to-confirm dialogs", () => {
  it("keeps the factory-reset token verbatim and gates the confirm button on it", () => {
    render(<FactoryResetDialog open onOpenChange={vi.fn()} />);

    // The whole reconstructed <Trans> sentence: a tag index drifting off its
    // <code> child reassembles the copy into fragments without failing
    // anything else in the suite.
    expect(screen.getByText(/to enable the confirm button/).textContent).toBe(
      "Type RESET to enable the confirm button. After the wipe completes you'll be asked to " +
        "quit and relaunch Kandev - the backend does not auto-restart.",
    );

    const input = screen.getByTestId("system-factory-reset-input");
    const confirm = screen.getByTestId("system-factory-reset-confirm") as HTMLButtonElement;
    expect(input.getAttribute("placeholder")).toBe("Type RESET to confirm");
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "reset" } });
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "RESET" } });
    expect(confirm.disabled).toBe(false);
  });

  it("keeps the restore token verbatim and interpolates the snapshot name", () => {
    render(<RestoreDialog open onOpenChange={vi.fn()} name="kandev-20260803.db" />);

    // The filename is a value; only the frame around it is copy.
    expect(screen.getByText(/over the current database/).textContent).toBe(
      "Restore kandev-20260803.db over the current database. After the staged copy is in place " +
        "you will be asked to quit and relaunch Kandev so the new data is loaded fresh - the " +
        "backend does not auto-restart.",
    );

    const input = screen.getByTestId("system-restore-input");
    const confirm = screen.getByTestId("system-restore-confirm") as HTMLButtonElement;
    expect(input.getAttribute("placeholder")).toBe("Type RESTORE to confirm");
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "RESTORE" } });
    expect(confirm.disabled).toBe(false);
  });
});

/**
 * The pseudo-locale oracle, at unit scale.
 *
 * `i18next/no-literal-string` runs in `mode: "jsx-only"`, so copy these dialogs
 * build outside JSX is invisible to lint and a clean lint is not proof they are
 * done. Under `pseudo` every catalog message is accented, so any word-like
 * ASCII left in the dialog is a literal that never reached the catalog — except
 * the confirmation token, which must stay verbatim for `typed === CONFIRM_TOKEN`
 * to be satisfiable.
 *
 * Accessibility copy is scanned too, and that is the point: the live pseudo
 * oracle only sees what is painted, so an `aria-label` is exactly the string
 * that survives a migration untranslated because nothing on screen looks wrong.
 */
describe("system type-to-confirm dialogs under the pseudo-locale", () => {
  const ACCENTED = /[À-ɏ]/;
  const WORDLIKE = /[A-Za-z]{4,}/;
  /** Attributes that carry copy, mirroring the eslint guard's include list. */
  const COPY_ATTRIBUTES = ["aria-label", "aria-description", "title", "placeholder", "alt"];

  function unlocalizedText(): string[] {
    const dialog = document.querySelector("[role=dialog]");
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

  /**
   * `Close` is `@kandev/ui`'s own `DialogContent` dismiss button. Localizing it
   * needs a strings-provider seam in that package (see docs/i18n.md, "Shared
   * UI"), so it is expected here rather than filtered out — if the package ever
   * gains that seam, this assertion is what tells us to drop it.
   */
  const UI_PACKAGE_CLOSE = "Close";

  it("leaves only the RESET token unaccented in the factory-reset dialog", () => {
    render(<FactoryResetDialog open onOpenChange={vi.fn()} />);
    // `<data-dir>/backups/` is a path placeholder and stays a value.
    expect(unlocalizedText().sort()).toEqual(["<data-dir>/backups/", UI_PACKAGE_CLOSE, "RESET"]);
    // Still typeable, and still announced, under a non-English locale.
    expect(screen.getByTestId("system-factory-reset-input").getAttribute("aria-label")).toContain(
      "RESET",
    );
  });

  it("leaves only the RESTORE token and the snapshot name unaccented", () => {
    render(<RestoreDialog open onOpenChange={vi.fn()} name="snapshot-1.db" />);
    expect(unlocalizedText().sort()).toEqual([UI_PACKAGE_CLOSE, "RESTORE", "snapshot-1.db"]);
    expect(screen.getByTestId("system-restore-input").getAttribute("aria-label")).toContain(
      "RESTORE",
    );
  });
});
