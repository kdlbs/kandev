import { expect, type Locator } from "@playwright/test";

export async function readTaskDescription(input: Locator): Promise<string> {
  return input.evaluate((element) => {
    if (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) {
      return element.value;
    }
    const serializedValue = element.getAttribute("data-task-description-value");
    if (serializedValue !== null) return serializedValue;
    return element.innerText;
  });
}

export async function expectTaskDescription(input: Locator, value: string): Promise<void> {
  await expect.poll(() => readTaskDescription(input)).toBe(value);
}
