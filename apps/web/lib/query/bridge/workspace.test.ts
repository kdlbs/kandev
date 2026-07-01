/* eslint-disable max-lines-per-function, sonarjs/no-duplicate-string */
import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerWorkspaceBridge } from "./workspace";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;
const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";

class FakeWebSocketClient {
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

describe("workspace query bridge", () => {
  it("invalidates workflow step query data for workflow step events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.steps(WORKFLOW_ID), [
      { id: "step-1", workflow_id: WORKFLOW_ID, name: "Todo", position: 1 },
    ]);
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), {
      workflow: { id: WORKFLOW_ID },
      steps: [],
      tasks: [],
    });

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.step.updated",
      payload: {
        step: {
          id: "step-1",
          workflow_id: WORKFLOW_ID,
          name: "Doing",
          position: 1,
          state: "active",
          color: "#00f",
        },
      },
    });

    expect(queryClient.getQueryState(qk.workflows.steps(WORKFLOW_ID))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.workflows.snapshot(WORKFLOW_ID))?.isInvalidated).toBe(true);

    registration.cleanup();
  });

  it("removes deleted workflows from cached workflow lists", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID), [
      { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Delete me" },
      { id: "workflow-2", workspace_id: WORKSPACE_ID, name: "Keep me" },
    ]);
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
      { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Delete me" },
      { id: "workflow-hidden", workspace_id: WORKSPACE_ID, name: "Hidden" },
    ]);
    queryClient.setQueryData(qk.workflows.all("workspace-2"), [
      { id: "workflow-other", workspace_id: "workspace-2", name: "Other workspace" },
    ]);

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.deleted",
      payload: { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID },
    });

    expect(queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID))).toEqual([
      expect.objectContaining({ id: "workflow-2" }),
    ]);
    expect(
      queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID, { includeHidden: true })),
    ).toEqual([expect.objectContaining({ id: "workflow-hidden" })]);
    expect(queryClient.getQueryData<unknown[]>(qk.workflows.all("workspace-2"))).toEqual([
      expect.objectContaining({ id: "workflow-other" }),
    ]);

    registration.cleanup();
  });

  it("patches cached workflow lists when workflow metadata changes", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID), [
      { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Old name", hidden: false },
      { id: "workflow-2", workspace_id: WORKSPACE_ID, name: "Keep me", hidden: false },
    ]);
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
      { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Old name", hidden: false },
      { id: "workflow-hidden", workspace_id: WORKSPACE_ID, name: "Hidden", hidden: true },
    ]);
    queryClient.setQueryData(qk.workflows.all("workspace-2"), [
      { id: WORKFLOW_ID, workspace_id: "workspace-2", name: "Other workspace" },
    ]);

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.updated",
      payload: {
        id: WORKFLOW_ID,
        workspace_id: WORKSPACE_ID,
        name: "Hidden now",
        hidden: true,
      },
    });

    expect(queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID))).toEqual([
      expect.objectContaining({ id: "workflow-2" }),
    ]);
    expect(
      queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID, { includeHidden: true })),
    ).toEqual([
      expect.objectContaining({ id: WORKFLOW_ID, name: "Hidden now", hidden: true }),
      expect.objectContaining({ id: "workflow-hidden" }),
    ]);
    expect(queryClient.getQueryData<unknown[]>(qk.workflows.all("workspace-2"))).toEqual([
      expect.objectContaining({ id: WORKFLOW_ID, name: "Other workspace" }),
    ]);

    registration.cleanup();
  });

  it("adds unhidden workflow updates back to cached visible workflow lists", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID), [
      { id: "workflow-2", workspace_id: WORKSPACE_ID, name: "Keep me", hidden: false },
    ]);
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
      { id: WORKFLOW_ID, workspace_id: WORKSPACE_ID, name: "Hidden", hidden: true },
    ]);

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.updated",
      payload: {
        id: WORKFLOW_ID,
        workspace_id: WORKSPACE_ID,
        name: "Visible now",
        hidden: false,
      },
    });

    expect(queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID))).toEqual([
      expect.objectContaining({ id: "workflow-2" }),
      expect.objectContaining({ id: WORKFLOW_ID, name: "Visible now", hidden: false }),
    ]);
    expect(
      queryClient.getQueryData<unknown[]>(qk.workflows.all(WORKSPACE_ID, { includeHidden: true })),
    ).toEqual([expect.objectContaining({ id: WORKFLOW_ID, name: "Visible now", hidden: false })]);

    registration.cleanup();
  });

  it("patches cached repository lists before invalidating", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), [
      { id: "repo-1", workspace_id: WORKSPACE_ID, name: "Old repo" },
      { id: "repo-2", workspace_id: WORKSPACE_ID, name: "Keep me" },
    ]);
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID, { includeScripts: true }), [
      { id: "repo-1", workspace_id: WORKSPACE_ID, name: "Old repo", scripts: [{ id: "script-1" }] },
    ]);
    queryClient.setQueryData(qk.workspaces.repositories("workspace-2"), [
      { id: "repo-1", workspace_id: "workspace-2", name: "Other workspace" },
    ]);

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "repository.updated",
      payload: {
        id: "repo-1",
        workspace_id: WORKSPACE_ID,
        name: "Renamed repo",
      },
    });

    expect(queryClient.getQueryData<unknown[]>(qk.workspaces.repositories(WORKSPACE_ID))).toEqual([
      expect.objectContaining({ id: "repo-1", name: "Renamed repo" }),
      expect.objectContaining({ id: "repo-2", name: "Keep me" }),
    ]);
    expect(
      queryClient.getQueryData<unknown[]>(
        qk.workspaces.repositories(WORKSPACE_ID, { includeScripts: true }),
      ),
    ).toEqual([
      expect.objectContaining({
        id: "repo-1",
        name: "Renamed repo",
        scripts: [{ id: "script-1" }],
      }),
    ]);
    expect(queryClient.getQueryData<unknown[]>(qk.workspaces.repositories("workspace-2"))).toEqual([
      expect.objectContaining({ id: "repo-1", name: "Other workspace" }),
    ]);
    expect(queryClient.getQueryState(qk.workspaces.repositories(WORKSPACE_ID))?.isInvalidated).toBe(
      true,
    );

    registration.cleanup();
  });

  it("removes deleted repositories from cached repository lists", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), [
      { id: "repo-1", workspace_id: WORKSPACE_ID, name: "Delete me" },
      { id: "repo-2", workspace_id: WORKSPACE_ID, name: "Keep me" },
    ]);

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "repository.deleted",
      payload: {
        id: "repo-1",
        workspace_id: WORKSPACE_ID,
      },
    });

    expect(queryClient.getQueryData<unknown[]>(qk.workspaces.repositories(WORKSPACE_ID))).toEqual([
      expect.objectContaining({ id: "repo-2" }),
    ]);

    registration.cleanup();
  });
});
