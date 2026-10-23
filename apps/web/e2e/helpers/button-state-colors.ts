import { expect, type Locator, type Page } from "@playwright/test";

export async function setButtonTheme(page: Page, dark: boolean) {
  await page.evaluate((value) => document.documentElement.classList.toggle("dark", value), dark);
}

export async function expectInheritedButtonForeground(button: Locator) {
  await expect
    .poll(() =>
      button.evaluate((element) => {
        const color = getComputedStyle(element).color;
        return [...element.querySelectorAll("svg, span")].every(
          (child) => getComputedStyle(child).color === color,
        );
      }),
    )
    .toBe(true);
}

export async function expectNeutralDisabledButton(button: Locator) {
  await expect(button).toBeDisabled();
  await expect(button).toHaveCSS("opacity", "1");
  await expect
    .poll(() =>
      button.evaluate((element) => {
        const probe = document.createElement("span");
        probe.style.cssText =
          "position:absolute;visibility:hidden;background:var(--muted);color:var(--muted-foreground)";
        element.append(probe);
        try {
          const actual = getComputedStyle(element);
          const target = getComputedStyle(probe);
          return actual.color === target.color && actual.backgroundColor === target.backgroundColor;
        } finally {
          probe.remove();
        }
      }),
    )
    .toBe(true);
  await expectInheritedButtonForeground(button);
  const contrast = await button.evaluate((element) => {
    const style = getComputedStyle(element);
    const luminance = (color: string) => {
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d")!;
      context.fillStyle = color;
      context.fillRect(0, 0, 1, 1);
      const channels = [...context.getImageData(0, 0, 1, 1).data].slice(0, 3).map((value) => {
        const channel = value / 255;
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
      });
      return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
    };
    const colors = [luminance(style.color), luminance(style.backgroundColor)].sort((a, b) => b - a);
    return (colors[0] + 0.05) / (colors[1] + 0.05);
  });
  expect(contrast).toBeGreaterThanOrEqual(3);
}

/** Hold a real save response so pending UI is observable without timing guesses. */
export async function holdFileSaveResponse(page: Page) {
  let release: (() => void) | undefined;
  let held = false;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    const ids = new Set<string>();
    socket.onMessage((message) => {
      if (typeof message === "string") {
        for (const line of message.split("\n")) {
          try {
            const frame = JSON.parse(line);
            if (frame.type === "request" && frame.action === "workspace.file.update")
              ids.add(frame.id);
          } catch {
            /* Forward non-JSON frames unchanged. */
          }
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      for (const line of message.split("\n")) {
        if (!line.trim()) continue;
        try {
          const frame = JSON.parse(line);
          if (frame.type === "response" && ids.delete(frame.id)) {
            held = true;
            release = () => {
              socket.send(line);
              held = false;
            };
            continue;
          }
        } catch {
          /* Forward non-JSON frames unchanged. */
        }
        socket.send(line);
      }
    });
  });
  return {
    isHeld: () => held,
    release: () => {
      release?.();
      release = undefined;
    },
  };
}
