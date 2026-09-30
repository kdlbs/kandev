import { expect } from "@playwright/test";

type ToolResult = { isError?: boolean; content: { type: string; text: string }[] };

export async function callPromptTool(baseUrl: string, name: string, args: Record<string, unknown>) {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json, text/event-stream",
  };
  const init = await fetch(`${baseUrl}/mcp`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 0,
      method: "initialize",
      params: {
        protocolVersion: "2024-11-05",
        capabilities: {},
        clientInfo: { name: "prompt-e2e", version: "1" },
      },
    }),
  });
  expect(init.ok).toBe(true);
  await init.text();
  const session = init.headers.get("mcp-session-id");
  if (session) headers["Mcp-Session-Id"] = session;
  const response = await fetch(`${baseUrl}/mcp`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 1,
      method: "tools/call",
      params: { name, arguments: args },
    }),
  });
  expect(response.ok).toBe(true);
  const text = await response.text();
  const data = text.startsWith("{")
    ? text
    : text
        .split("\n")
        .find((line) => line.startsWith("data: "))
        ?.slice(6);
  expect(data).toBeTruthy();
  const rpc = JSON.parse(data!) as { result: ToolResult; error?: unknown };
  if (session) await fetch(`${baseUrl}/mcp`, { method: "DELETE", headers });
  expect(rpc.error).toBeUndefined();
  return rpc.result;
}
