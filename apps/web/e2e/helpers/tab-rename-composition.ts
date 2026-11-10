import { expect, type Locator } from "@playwright/test";

/** Browser-dispatched events exercise input handling, not physical OS IME integration. */
export async function composeTabRename(input: Locator) {
  await expect(input).toHaveCount(1);
  await expect(input).toBeFocused();
  await input.dispatchEvent("compositionstart", { data: "" });
  await input.fill("日本");
  for (const key of [
    { key: "Enter", isComposing: true, keyCode: 13 },
    { key: "Enter", isComposing: false, keyCode: 229 },
    { key: "Escape", isComposing: true, keyCode: 27 },
  ]) {
    await input.dispatchEvent("keydown", { ...key, bubbles: true, cancelable: true });
    await expect(input).toHaveCount(1);
    await expect(input).toHaveValue("日本");
    await expect(input).toBeFocused();
  }
  await input.dispatchEvent("compositionend", { data: "日本語" });
  await input.fill("日本語");
}
