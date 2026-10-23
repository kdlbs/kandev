import { expect, type Page } from "@playwright/test";

// Match the rendered engine and Monaco's browser-platform keyboard profile.
export async function markdownModifier(page: Page): Promise<"Meta" | "Control"> {
  return page.evaluate(() => (/Macintosh|Mac OS X/.test(navigator.userAgent) ? "Meta" : "Control"));
}

export async function markdownDocumentEnd(page: Page): Promise<string> {
  return (await markdownModifier(page)) === "Meta" ? "Meta+ArrowDown" : "Control+End";
}

// CodeMirror treats iOS Safari as Mac, including when emulated on a Linux host.
export async function codeMirrorDocumentEnd(page: Page): Promise<string> {
  return page.evaluate(() => {
    const ios =
      /Apple Computer/.test(navigator.vendor) &&
      (/Mobile\/\w+/.test(navigator.userAgent) || navigator.maxTouchPoints > 2);
    return ios || /Mac/.test(navigator.platform) ? "Meta+ArrowDown" : "Control+End";
  });
}

async function sourceState(page: Page) {
  return page.getByRole("textbox", { name: "Editor content", exact: true }).evaluate((element) => {
    const monaco = (window as Window & { monaco?: typeof import("monaco-editor") }).monaco;
    const editor = monaco?.editor
      .getEditors()
      .find((candidate) => candidate.getDomNode()?.contains(element));
    const model = editor?.getModel();
    const selection = editor?.getSelection();
    if (!model || !selection) throw new Error("Source editor is not ready");
    const value = model.getValue();
    return {
      value,
      prefix: value.slice(0, model.getOffsetAt(selection.getStartPosition())),
      suffix: value.slice(model.getOffsetAt(selection.getEndPosition())),
    };
  });
}

// Wait for Monaco to accept each command before sending the next native input.
export async function typeMarkdownSource(page: Page, content: string): Promise<void> {
  const initial = await sourceState(page);
  const lines = content.split("\n");
  let inserted = lines[0];
  await page.keyboard.type(inserted);
  const expectSource = () =>
    expect
      .poll(async () => (await sourceState(page)).value)
      .toBe(initial.prefix + inserted + initial.suffix);
  await expectSource();
  for (const line of lines.slice(1)) {
    // Enter accepts a visible suggestion, including a word already in the buffer.
    await page.keyboard.press("Escape");
    await expect(page.locator(".monaco-editor .suggest-widget.visible")).toHaveCount(0);
    await page.keyboard.press("Enter");
    inserted += "\n";
    await expectSource();
    if (line) {
      await page.keyboard.type(line);
      inserted += line;
      await expectSource();
    }
  }
}
