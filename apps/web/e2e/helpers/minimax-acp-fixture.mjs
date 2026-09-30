import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";

// A deterministic ACP peer with the published MiniMax 0.5.10 wire shape.
// It exercises Kandev's native registration; it never calls a model provider.
const root = path.dirname(path.dirname(process.argv[1]));
const authenticated = path.join(root, "authenticated");
if (process.argv.includes("--version")) {
  console.log("0.5.10");
  process.exit(0);
}
if (process.argv.includes("login")) {
  fs.writeFileSync(authenticated, "fixture login");
  console.log("MiniMax fixture login complete");
  process.exit(0);
}
const models = [
  ["m:minimax:MiniMax-M3:v:thinking", "MiniMax-M3 · thinking"],
  ["m:minimax:MiniMax-M3.1-Flash-Preview:v:thinking", "M3.1-Flash-Preview · thinking"],
  ["m:minimax:MiniMax-M2.7-highspeed:v:thinking", "MiniMax-M2.7-highspeed"],
  ["m:minimax:MiniMax-M2.7:v:thinking", "MiniMax-M2.7"],
];
let model = models[0][0];
function configOptions() {
  return [
    {
      type: "select",
      id: "model",
      category: "model",
      name: "Model",
      currentValue: model,
      options: models.map(([value, name]) => ({ value, name })),
    },
  ];
}
const send = (frame) => process.stdout.write(JSON.stringify({ jsonrpc: "2.0", ...frame }) + "\n");
const lines = readline.createInterface({ input: process.stdin });
lines.on("line", (line) => {
  const { id, method, params = {} } = JSON.parse(line);
  if (id === undefined) return;
  let result = {};
  if (method === "initialize") {
    result = {
      protocolVersion: 1,
      agentInfo: { name: "minimax-code", version: "0.5.10" },
      agentCapabilities: {
        loadSession: true,
        mcpCapabilities: { http: true, sse: true },
        promptCapabilities: { image: false, audio: false, embeddedContext: false },
      },
    };
  } else if (method === "session/new" || method === "session/load") {
    if (!fs.existsSync(authenticated)) {
      send({
        id,
        error: {
          code: -32000,
          message: "Authentication required: Run `mcode login` and try again.",
        },
      });
      return;
    }
    result = { sessionId: params.sessionId ?? "fixture-session", configOptions: configOptions() };
  } else if (method === "session/set_config_option") {
    if (!models.some(([value]) => value === params.value)) {
      send({ id, error: { code: -32602, message: "Unknown model selection" } });
      return;
    }
    model = params.value;
    result = { configOptions: configOptions() };
  } else if (method === "session/prompt") {
    send({
      method: "session/update",
      params: {
        sessionId: params.sessionId,
        update: {
          sessionUpdate: "agent_message_chunk",
          content: { type: "text", text: "MiniMax fixture response" },
        },
      },
    });
    result = { stopReason: "end_turn" };
  }
  send({ id, result });
});
