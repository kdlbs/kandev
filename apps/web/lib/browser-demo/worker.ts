/// <reference lib="webworker" />
import { type DemoWorkerRequest } from "./protocol";
import {
  conversationRuntime,
  initializeDemoRuntime,
  post,
  scope,
  socketUrls,
  terminalInputBySocket,
} from "./runtime-state";
import { handleSocketRequest } from "./socket-runtime";
import { handleHttp } from "./task-http-runtime";
import {
  handleTerminalInput,
  isTerminalSocket,
  terminalOutput,
  terminalWelcome,
} from "./terminal-runtime";

export { handleSocketRequest } from "./socket-runtime";
export { handleHttp } from "./task-http-runtime";
scope.onmessage = (event: MessageEvent<DemoWorkerRequest>) => {
  const message = event.data;
  if (message.kind === "init") {
    initializeDemoRuntime(message);
    return;
  }
  if (message.kind === "http") {
    void handleHttp(message.request).then((response) =>
      post({ kind: "http-result", id: message.id, response }),
    );
    return;
  }
  if (message.kind === "ws-open") {
    socketUrls.set(message.socketId, message.url);
    post({ kind: "ws-event", socketId: message.socketId, event: "open" });
    if (isTerminalSocket(message.url)) {
      setTimeout(() => terminalOutput(message.socketId, terminalWelcome(message.url)), 25);
    }
    return;
  }
  if (message.kind === "ws-close") {
    socketUrls.delete(message.socketId);
    conversationRuntime.close(message.socketId);
    terminalInputBySocket.delete(message.socketId);
    post({ kind: "ws-event", socketId: message.socketId, event: "close" });
    return;
  }
  if (message.kind === "ws-send") {
    if (isTerminalSocket(socketUrls.get(message.socketId) ?? "")) {
      handleTerminalInput(message.socketId, message.data);
    } else if (typeof message.data === "string") {
      handleSocketRequest(message.socketId, message.data);
    }
  }
};
