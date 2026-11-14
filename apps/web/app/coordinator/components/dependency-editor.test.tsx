import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const getMock = vi.fn();
const putMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-merge-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/coordinator-merge-api")>()),
  getDependencies: (...a: unknown[]) => getMock(...a),
  putDependencies: (...a: unknown[]) => putMock(...a),
}));

import { DependencyEditor } from "./dependency-editor";

const SELF_TESTID = "queue-deps-self";

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

function mount(canManage = true) {
  return render(
    <DependencyEditor workspaceId="w1" coordinatorId="c1" taskId="self" canManage={canManage} />,
  );
}

async function add(value: string) {
  fireEvent.change(screen.getByTestId("queue-deps-input"), { target: { value } });
  fireEvent.click(screen.getByTestId("queue-deps-add"));
}

describe("DependencyEditor", () => {
  it("refuses a duplicate and the card itself", async () => {
    getMock.mockResolvedValue({ refs: [{ task: "a" }], revision: 1 });
    mount();
    await screen.findByTestId(SELF_TESTID);
    await add("a");
    expect(screen.getByTestId("queue-deps-message").textContent).toContain("already listed");
    await add("self");
    expect(screen.getByTestId("queue-deps-message").textContent).toContain("own predecessor");
    expect(putMock).not.toHaveBeenCalled();
  });

  it("saves the whole set with the revision it was read at", async () => {
    getMock.mockResolvedValue({ refs: [], revision: 4 });
    putMock.mockResolvedValue({ refs: [{ task: "b" }], revision: 5 });
    mount();
    await screen.findByTestId(SELF_TESTID);
    await add("b");
    fireEvent.click(screen.getByTestId("queue-deps-save"));
    await screen.findByTestId("queue-deps-saved");
    expect(putMock).toHaveBeenCalledWith("w1", "c1", "self", 4, [{ task: "b" }]);
  });

  it("shows a stale 409 conflict and reloads the stored set", async () => {
    getMock
      .mockResolvedValueOnce({ refs: [], revision: 1 })
      .mockResolvedValueOnce({ refs: [{ task: "z" }], revision: 2 });
    putMock.mockRejectedValue(new ApiError("x", 409, { code: "revision_conflict" }));
    mount();
    await screen.findByTestId(SELF_TESTID);
    await add("b");
    fireEvent.click(screen.getByTestId("queue-deps-save"));
    await waitFor(() =>
      expect(screen.getByTestId("queue-deps-message").textContent).toContain("changed elsewhere"),
    );
    await waitFor(() => expect(screen.getByTestId("queue-deps-list").textContent).toContain("z"));
  });

  it("offers no editing without manage rights", async () => {
    getMock.mockResolvedValue({ refs: [{ task: "a" }], revision: 1 });
    mount(false);
    await screen.findByTestId(SELF_TESTID);
    expect(screen.queryByTestId("queue-deps-input")).toBeNull();
    expect(screen.queryByTestId("queue-deps-save")).toBeNull();
  });
});
