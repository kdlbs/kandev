import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { SecretDeleteConfirmation } from "./secrets-delete-dialog";

const secret: SecretListItem = {
  id: "secret-1",
  name: "OpenAI Production Key",
  has_value: true,
  created_at: "",
  updated_at: "",
};

const CONFIRM_POPOVER_TEST_ID = "secret-delete-confirm-popover";
const CONFIRM_TEST_ID = "secret-delete-confirm";

function ConfirmationHarness({ onConfirm = vi.fn() }: { onConfirm?: () => void | Promise<void> }) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(true);
  return (
    <>
      <button ref={anchorRef} type="button">
        Delete secret
      </button>
      <SecretDeleteConfirmation
        secret={secret}
        open={open}
        anchorRef={anchorRef}
        onOpenChange={setOpen}
        onCancel={() => setOpen(false)}
        onConfirm={onConfirm}
      />
    </>
  );
}

function descriptionText(): string {
  const description = screen.getByText(secret.name).closest('[data-slot="popover-description"]');
  if (!description) throw new Error("secret name is not inside the popover description");
  return description.textContent ?? "";
}

afterEach(async () => {
  cleanup();
  await activateLocale("en");
});

describe("SecretDeleteConfirmation", () => {
  it("uses a local non-blocking confirmation surface", async () => {
    render(<ConfirmationHarness />);

    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).not.toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" })),
    );
  });

  it("renders localized copy around the name without exposing secret material", () => {
    render(<ConfirmationHarness />);

    expect(descriptionText()).toBe(
      `This will permanently remove ${secret.name}. This action cannot be undone.`,
    );
    expect(descriptionText()).not.toContain("secret-value");
  });

  it("keeps the secret name verbatim when the locale changes", async () => {
    await activateLocale("pseudo");
    render(<ConfirmationHarness />);

    expect(descriptionText()).toMatch(/[À-ɏ]/);
    expect(descriptionText()).not.toContain("This will permanently remove");
    expect(descriptionText()).toContain(secret.name);
  });

  it("closes before invoking confirmation and returns focus on cancel", async () => {
    const onConfirm = vi.fn(() => new Promise<void>(() => {}));
    render(<ConfirmationHarness onConfirm={onConfirm} />);

    const cancel = screen.getByRole("button", { name: "Cancel" });
    const anchor = screen.getByRole("button", { name: "Delete secret" });
    fireEvent.click(cancel);

    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(anchor));
  });

  it("invokes confirmation once after the shell closes", async () => {
    let shellClosed = false;
    const onConfirm = vi.fn(() => {
      shellClosed = screen.queryByTestId(CONFIRM_POPOVER_TEST_ID) === null;
    });
    render(<ConfirmationHarness onConfirm={onConfirm} />);

    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(CONFIRM_TEST_ID),
    );

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(shellClosed).toBe(true);
  });
});
