import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { DeleteSecretDialog } from "./secrets-settings";

/**
 * A secret's name is user data. It reaches the dialog as an interpolated value
 * and must never become a catalog key — the pseudo locale is the oracle here:
 * the sentence around the name changes, the name itself must not.
 */
const secret: SecretListItem = {
  id: "secret-1",
  name: "OpenAI Production Key",
  has_value: true,
  created_at: "",
  updated_at: "",
};

/**
 * The name renders inside a `<span>`, so testing-library's text matcher — which
 * joins only an element's direct text-node children — never sees the sentence
 * whole. Read the description element's own `textContent` instead.
 */
function descriptionText(): string {
  const description = screen.getByText(secret.name).closest("p");
  // Fail on the broken query rather than on an empty string comparison, which
  // would point every assertion below at the wrong cause.
  if (!description) throw new Error("secret name is not inside the <p> DialogDescription");
  return description.textContent ?? "";
}

afterEach(async () => {
  cleanup();
  await activateLocale("en");
});

describe("DeleteSecretDialog", () => {
  it("renders the confirmation copy around the secret name", () => {
    render(
      <DeleteSecretDialog target={secret} onClose={vi.fn()} onConfirm={vi.fn()} isBusy={false} />,
    );

    expect(descriptionText()).toBe(
      `This will permanently remove ${secret.name}. This action cannot be undone.`,
    );
  });

  it("keeps the secret name verbatim when the locale changes", async () => {
    await activateLocale("pseudo");
    render(
      <DeleteSecretDialog target={secret} onClose={vi.fn()} onConfirm={vi.fn()} isBusy={false} />,
    );

    // The surrounding sentence is catalog copy, so pseudo accents it.
    expect(descriptionText()).toMatch(/[À-ɏ]/);
    expect(descriptionText()).not.toContain("This will permanently remove");
    // The name is user data — byte-identical under any locale.
    expect(descriptionText()).toContain(secret.name);
  });
});
