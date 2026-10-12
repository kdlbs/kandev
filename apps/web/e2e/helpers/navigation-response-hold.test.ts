import { expect, it, vi } from "vitest";
import { NavigationResponseGate } from "./navigation-response-hold";

const request = (id: string, path: string) =>
  JSON.stringify({
    id,
    type: "request",
    action: "workspace.tree.get",
    payload: { session_id: "A", path },
  });
const response = (id: string) =>
  JSON.stringify({ id, type: "response", action: "workspace.tree.get", payload: { root: null } });

it("holds only correlated replies and forwards unrelated batched frames", () => {
  const gate = new NavigationResponseGate();
  const send = vi.fn();
  gate.hold((frame) => frame.payload.path === "held");
  gate.recordRequests(`${request("one", "held")}\n${request("two", "available")}`);
  gate.forwardResponses(`${response("one")}\n${response("two")}\nnot-json`, send);
  expect(gate.heldCount()).toBe(1);
  expect(send).toHaveBeenCalledWith(`${response("two")}\nnot-json`);
  gate.release();
  expect(send).toHaveBeenLastCalledWith(response("one"));
  expect(gate.heldCount()).toBe(0);
});

it("preserves untouched messages and can fail a held response once", () => {
  const gate = new NavigationResponseGate();
  const send = vi.fn();
  const untouched = `  ${response("unrelated")}\n`;
  gate.forwardResponses(untouched, send);
  expect(send).toHaveBeenCalledWith(untouched);
  gate.hold(() => true);
  gate.recordRequests(request("one", "held"));
  gate.forwardResponses(response("one"), send);
  gate.release("temporary folder failure");
  expect(JSON.parse(send.mock.lastCall![0])).toMatchObject({
    id: "one",
    type: "error",
    payload: { message: "temporary folder failure" },
  });
  gate.recordRequests(request("two", "held"));
  gate.forwardResponses(response("two"), send);
  expect(send).toHaveBeenLastCalledWith(response("two"));
  expect(gate.requests).toHaveLength(2);
});
