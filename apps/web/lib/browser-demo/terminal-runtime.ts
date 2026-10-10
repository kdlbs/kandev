import { post, terminalInputBySocket } from "./runtime-state";
import { normalizeFilePath, workspaceFilesForSession } from "./workspace-runtime";

export function isTerminalSocket(url: string) {
  try {
    return new URL(url, "https://demo.kandev.com").pathname.includes("/terminal/");
  } catch {
    return false;
  }
}

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function terminalWelcome(url: string) {
  const isAgent = url.includes("/terminal/session/");
  const heading = isAgent ? "Mock agent terminal" : "Acme Platform workspace";
  return (
    `\u001b[2J\u001b[H\u001b[1;36m${heading}\u001b[0m\r\n` +
    "Browser demo shell. Commands run against simulated workspace data.\r\n\r\n" +
    "demo@acme-web ~/acme-web $ "
  );
}

export function handleTerminalInput(socketId: string, data: string | ArrayBuffer) {
  const bytes = typeof data === "string" ? null : new Uint8Array(data);
  if (bytes?.[0] === 0x01) return;
  const input = typeof data === "string" ? data : new TextDecoder().decode(new Uint8Array(data));
  const previous = terminalInputBySocket.get(socketId) ?? "";
  let current = previous;
  for (const character of input) {
    if (character === "\r" || character === "\n") {
      terminalOutput(socketId, `\r\n${terminalCommandResult(current)}demo@acme-web ~/acme-web $ `);
      current = "";
    } else if (character === "\u007f") {
      current = current.slice(0, -1);
    } else {
      current += character;
      terminalOutput(socketId, character);
    }
  }
  terminalInputBySocket.set(socketId, current);
}

// i18n-exempt: browser demo fixture data is intentionally literal demo content
export function terminalCommandResult(command: string) {
  const normalized = command.trim();
  if (!normalized) return "";
  if (normalized === "pwd") return "/demo/acme-web\r\n";
  if (normalized === "ls") return "README.md  package.json  src  tests\r\n";
  if (normalized === "git status")
    return "On branch kandev/audit-logging\r\nChanges not staged for commit:\r\n  modified: src/api/audit.ts\r\n";
  if (normalized === "pnpm test")
    return "✓ tests/audit-log.test.tsx (1 test)\r\nTest Files  1 passed (1)\r\n";
  if (normalized.startsWith("cat ")) {
    const content = workspaceFilesForSession("")[normalizeFilePath(normalized.slice(4))];
    return content === undefined
      ? `cat: file not found\r\n`
      : `${content.replaceAll("\n", "\r\n")}\r\n`;
  }
  return `command not available in browser demo: ${normalized}\r\n`;
}

export function terminalOutput(socketId: string, data: string) {
  post({ kind: "ws-event", socketId, event: "message", data });
}
