import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";

const mockRequest = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

import { DeleteSessionDialog } from "./session-tab-menu";

describe("DeleteSessionDialog", () => {
  beforeEach(() => {
    mockRequest.mockReset();
    mockRequest.mockResolvedValue({ snapshots: [] });
  });

  afterEach(cleanup);

  it("does not claim only conversation history is removed", () => {
    render(
      <DeleteSessionDialog
        open
        onOpenChange={() => {}}
        isPrimary={false}
        sessionCount={2}
        sessionId="session-1"
        onConfirm={() => {}}
      />,
    );
    const description = screen.getByText(/permanently delete/i);
    expect(description.textContent?.toLowerCase()).not.toMatch(
      /only.*conversation history|conversation history.*only/,
    );
  });

  it("requests the session's git snapshots keyed by the given session id", async () => {
    render(
      <DeleteSessionDialog
        open
        onOpenChange={() => {}}
        isPrimary={false}
        sessionCount={1}
        sessionId="session-warn"
        onConfirm={() => {}}
      />,
    );
    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledWith(
        "session.git.snapshots",
        expect.objectContaining({ session_id: "session-warn" }),
      );
    });
  });

  it("shows uncommitted-file and unpushed-commit counts before the confirm control is used", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [{ branch: "main", ahead: 2, files: { "a.ts": {}, "b.ts": {}, "c.ts": {} } }],
    });
    render(
      <DeleteSessionDialog
        open
        onOpenChange={() => {}}
        isPrimary={false}
        sessionCount={1}
        sessionId="session-dirty"
        onConfirm={() => {}}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("session-delete-uncommitted-warning").textContent).toContain("3");
    });
    expect(screen.getByTestId("session-delete-unpushed-warning").textContent).toContain("2");
    // Both counts are visible before the confirm control is ever used —
    // nothing gates the delete button on the fetch resolving.
    expect(screen.getByRole("button", { name: /delete/i })).toBeTruthy();
  });

  it("shows no warning for a session that is clean and level with its remote", async () => {
    mockRequest.mockResolvedValue({ snapshots: [{ branch: "main", ahead: 0, files: {} }] });
    render(
      <DeleteSessionDialog
        open
        onOpenChange={() => {}}
        isPrimary={false}
        sessionCount={1}
        sessionId="session-clean"
        onConfirm={() => {}}
      />,
    );

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalled();
    });
    expect(screen.queryByTestId("session-delete-uncommitted-warning")).toBeNull();
    expect(screen.queryByTestId("session-delete-unpushed-warning")).toBeNull();
  });

  it("does not fetch snapshots when no session id is known", () => {
    render(
      <DeleteSessionDialog
        open
        onOpenChange={() => {}}
        isPrimary={false}
        sessionCount={1}
        onConfirm={() => {}}
      />,
    );
    expect(mockRequest).not.toHaveBeenCalled();
  });
});
