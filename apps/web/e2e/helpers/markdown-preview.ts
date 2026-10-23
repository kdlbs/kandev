import { expect, type Locator, type Page } from "@playwright/test";

export async function selectMobileMarkdownMode(
  page: Page,
  viewer: Locator,
  mode: "preview" | "edit" | "source",
  expectedMode: "preview" | "edit" | "source" = mode,
): Promise<void> {
  const menu = page.getByTestId("mobile-markdown-mode-menu");
  const trigger = viewer.getByTestId("mobile-markdown-mode-trigger");
  await expect(menu).toBeHidden();
  await trigger.tap();
  await menu.getByTestId(`mobile-markdown-mode-${mode}`).tap();
  await expect(menu).toBeHidden();
  await expect(trigger).toHaveAttribute("data-mode", expectedMode);
}

export async function selectMarkdownPreviewText(target: Locator): Promise<void> {
  await target.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    const selection = window.getSelection();
    if (!selection) throw new Error("Browser selection is unavailable");
    selection.removeAllRanges();
    selection.addRange(range);

    const rect = element.getBoundingClientRect();
    element.dispatchEvent(
      new MouseEvent("mouseup", {
        bubbles: true,
        clientX: rect.left + rect.width / 2,
        clientY: rect.bottom,
      }),
    );
  });
}

export async function selectMarkdownPreviewRange(start: Locator, end: Locator): Promise<void> {
  await start.evaluate(
    (startElement, endElement) => {
      const range = document.createRange();
      range.setStartBefore(startElement);
      range.setEndAfter(endElement);
      const selection = window.getSelection();
      if (!selection) throw new Error("Browser selection is unavailable");
      selection.removeAllRanges();
      selection.addRange(range);

      const rect = endElement.getBoundingClientRect();
      endElement.dispatchEvent(
        new MouseEvent("mouseup", {
          bubbles: true,
          clientX: rect.left + rect.width / 2,
          clientY: rect.bottom,
        }),
      );
    },
    await end.elementHandle(),
  );
}
