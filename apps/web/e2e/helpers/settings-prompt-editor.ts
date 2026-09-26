import { type Locator, type Page, expect } from "@playwright/test";

type PromptEditorOptions = {
  touch?: boolean;
};

type CompletionOptions = {
  keyboard?: boolean;
  touch?: boolean;
};

/** Return the Monaco host inside a SettingsPromptEditor wrapper. */
export function monacoPromptEditor(editor: Locator): Locator {
  return editor.locator(".monaco-editor");
}

/** Focus Monaco's EditContext target, which is the input surface in this build. */
export async function focusPromptEditor(
  editor: Locator,
  { touch = false }: PromptEditorOptions = {},
): Promise<Locator> {
  const monaco = monacoPromptEditor(editor);
  await expect(monaco).toBeVisible({ timeout: 10_000 });
  if (touch) await monaco.tap();
  else await monaco.click();
  const editContext = monaco.locator(".native-edit-context");
  await expect(editContext).toBeFocused({ timeout: 5_000 });
  return monaco;
}

export async function replacePromptEditor(
  page: Page,
  editor: Locator,
  value: string,
  options?: PromptEditorOptions,
): Promise<Locator> {
  const monaco = await focusPromptEditor(editor, options);
  await page.keyboard.press("Control+A");
  await page.keyboard.press("Backspace");
  await page.keyboard.type(value);
  return monaco;
}

export async function waitForPromptInStore(page: Page, promptName: string): Promise<void> {
  await page.waitForFunction(
    (name) => {
      const store = (
        window as Window & {
          __KANDEV_E2E_STORE__?: {
            getState: () => {
              prompts: {
                loaded: boolean;
                items: Array<{ name: string }>;
              };
            };
          };
        }
      ).__KANDEV_E2E_STORE__;
      const prompts = store?.getState().prompts;
      return prompts?.loaded === true && prompts.items.some((prompt) => prompt.name === name);
    },
    promptName,
    {
      timeout: 10_000,
      message: `saved prompt ${promptName} was not loaded into the settings store`,
    },
  );
}

export async function acceptPromptCompletion(
  page: Page,
  label: string,
  { keyboard = false, touch = false }: CompletionOptions = {},
): Promise<void> {
  const suggestWidget = page.locator(".suggest-widget:visible").last();
  await expect(suggestWidget).toBeVisible({ timeout: 5_000 });
  const suggestion = suggestWidget.locator(".monaco-list-row").filter({ hasText: label }).first();
  await expect(suggestion).toBeVisible({ timeout: 5_000 });
  if (!keyboard) {
    if (touch) await suggestion.tap();
    else await suggestion.click();
    return;
  }

  const index = Number((await suggestion.getAttribute("data-index")) ?? "0");
  await page.keyboard.press("Home");
  for (let step = 0; step < index; step += 1) {
    await page.keyboard.press("ArrowDown");
  }
  await page.keyboard.press("Enter");
}

export function promptEditorText(editor: Locator): Locator {
  return monacoPromptEditor(editor).locator(".view-lines");
}
