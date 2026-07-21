import type { Locator } from "@playwright/test";

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
