import { expect, type Page } from "@playwright/test";
import { readTerminalBuffer } from "./mobile-terminal-helpers";

// Encode the program so its output markers cannot be mistaken for echoed input.
const program = `import curses
def read_key(screen):
    screen.keypad(True)
    screen.addstr(0, 0, "CURSOR_READY")
    screen.refresh()
    return screen.getch()
key = curses.wrapper(read_key)
print("CURSOR_RESULT:", curses.keyname(key).decode("ascii"), flush=True)
`;

export async function startCursorProgram(page: Page): Promise<void> {
  const encoded = Buffer.from(program).toString("base64");
  await page.keyboard.type(`python3 -c 'import base64;exec(base64.b64decode("${encoded}"))'`);
  await page.keyboard.press("Enter");
  await expect.poll(() => readTerminalBuffer(page), { timeout: 15_000 }).toContain("CURSOR_READY");
}

export async function expectCursorResult(page: Page, key: string): Promise<void> {
  await expect
    .poll(() => readTerminalBuffer(page), { timeout: 10_000 })
    .toContain(`CURSOR_RESULT: ${key}`);
}
