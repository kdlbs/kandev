import fs from "node:fs";
import path from "node:path";
import readline from "node:readline";

const observationFile = process.argv[2];
if (!observationFile) {
  throw new Error("profile mode fixture requires an observation file path");
}

const fixtureModel = "fixture-model";
let mode = "workspace";
let model = fixtureModel;
const modes = [
  { id: "dont_ask", name: "Don't Ask" },
  { id: "ask", name: "Ask" },
];
const modeChoices = [
  { value: "session", name: "Don't Ask" },
  { value: "workspace", name: "Ask" },
];

function configOptions() {
  return [
    {
      type: "select",
      id: "edit_approval_policy",
      category: "mode",
      name: "Approval policy",
      currentValue: mode,
      options: modeChoices,
    },
    {
      type: "select",
      id: "model",
      category: "model",
      name: "Model",
      currentValue: model,
      options: [{ value: fixtureModel, name: "Fixture Model" }],
    },
  ];
}

function send(frame) {
  process.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", ...frame })}\n`);
}

function sendError(id, code, message) {
  send({ id, error: { code, message } });
}

function initialize(id, params) {
  send({
    id,
    result: {
      protocolVersion: params.protocolVersion ?? 1,
      agentInfo: { name: "profile-mode-fixture", version: "1.0.0" },
      agentCapabilities: {
        loadSession: false,
        mcpCapabilities: { http: true, sse: true },
        promptCapabilities: {},
      },
    },
  });
}

function startSession(id, params) {
  mode = "workspace";
  model = fixtureModel;
  send({
    id,
    result: {
      sessionId: params.sessionId ?? "profile-mode-fixture-session",
      modes: { currentModeId: "dont_ask", availableModes: modes },
      configOptions: configOptions(),
    },
  });
}

function setConfigOption(id, params) {
  if (
    params.configId === "edit_approval_policy" &&
    modeChoices.some(({ value }) => value === params.value)
  ) {
    mode = params.value;
  } else if (params.configId === "model" && params.value === fixtureModel) {
    model = params.value;
  } else {
    sendError(id, -32602, "Unknown mode config selection");
    return;
  }
  send({ id, result: { configOptions: configOptions() } });
}

function prompt(id, params) {
  fs.mkdirSync(path.dirname(observationFile), { recursive: true });
  fs.appendFileSync(observationFile, `${JSON.stringify({ mode, model, prompt: params.prompt })}\n`);
  send({
    method: "session/update",
    params: {
      sessionId: params.sessionId,
      update: {
        sessionUpdate: "agent_message_chunk",
        content: { type: "text", text: "ACP profile mode fixture response" },
      },
    },
  });
  send({ id, result: { stopReason: "end_turn" } });
}

const handlers = new Map([
  ["initialize", initialize],
  ["session/new", startSession],
  ["session/load", startSession],
  ["session/set_config_option", setConfigOption],
  [
    "session/set_mode",
    (id) => sendError(id, -32601, "The fixture exposes modes through config options only"),
  ],
  ["session/prompt", prompt],
  ["session/cancel", (id) => send({ id, result: {} })],
  ["session/close", (id) => send({ id, result: {} })],
]);

const lines = readline.createInterface({ input: process.stdin });
lines.on("line", (line) => {
  const { id, method, params = {} } = JSON.parse(line);
  if (id === undefined) return;
  const handler = handlers.get(method);
  if (handler) {
    handler(id, params);
    return;
  }
  sendError(id, -32601, `Unsupported fixture method ${method}`);
});
