import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

const previewShareMock = vi.fn();
const createShareMock = vi.fn();
const revokeShareMock = vi.fn();
const listSharesMock = vi.fn();

vi.mock("@/lib/api/domains/share-api", () => ({
  previewShare: (...args: unknown[]) => previewShareMock(...args),
  createShare: (...args: unknown[]) => createShareMock(...args),
  revokeShare: (...args: unknown[]) => revokeShareMock(...args),
  listShares: (...args: unknown[]) => listSharesMock(...args),
}));

import { ShareDialog } from "./share-dialog";

const SAMPLE_SNAPSHOT = {
  version: 1,
  exported_at: "2026-05-21T12:00:00.000Z",
  task: { title: "Investigate flake" },
  session: {
    agent_type: "claude-acp",
    model: "claude-opus-4-7",
    executor_type: "local_docker",
    started_at: "2026-05-21T11:55:00.000Z",
    completed_at: "2026-05-21T12:00:00.000Z",
  },
  messages: [
    { role: "user" as const, ts: "now", blocks: [{ kind: "text" as const, text: "hello" }] },
  ],
  redaction: { applied_rules: ["abs-path"] },
};

beforeEach(() => {
  previewShareMock.mockReset();
  createShareMock.mockReset();
  revokeShareMock.mockReset();
  listSharesMock.mockReset();
  listSharesMock.mockResolvedValue({ shares: [] });
});

describe("ShareDialog", () => {
  it("loads the preview and shows the warning + publish button", async () => {
    previewShareMock.mockResolvedValueOnce(SAMPLE_SNAPSHOT);
    render(<ShareDialog open={true} onOpenChange={() => {}} taskId="t-1" sessionId="sess-1" />);

    await waitFor(() => expect(previewShareMock).toHaveBeenCalled());
    expect(await screen.findByText(/Anyone with this link can view/i)).toBeTruthy();
    expect(await screen.findByRole("button", { name: /Publish to GitHub Gist/i })).toBeTruthy();
    // Message body and redaction summary render.
    expect(screen.getByText(/hello/)).toBeTruthy();
    expect(screen.getByText(/Redacted: abs-path/)).toBeTruthy();
  });

  it("renders text blocks as markdown in the preview", async () => {
    previewShareMock.mockResolvedValueOnce({
      ...SAMPLE_SNAPSHOT,
      messages: [
        {
          role: "assistant" as const,
          ts: "now",
          blocks: [
            {
              kind: "text" as const,
              text: "## Summary\n\n**Pushed** successfully.\n\n- tests passed\n- lint passed\n\n[docs](https://example.com)\n\n![tracker](https://attacker.example/pixel)",
            },
          ],
        },
      ],
    });

    render(<ShareDialog open={true} onOpenChange={() => {}} taskId="t-1" sessionId="sess-1" />);

    expect(await screen.findByRole("heading", { level: 2, name: "Summary" })).toBeTruthy();
    expect(screen.getByText("Pushed").tagName).toBe("STRONG");
    expect(screen.getByRole("list").children).toHaveLength(2);
    expect(screen.queryByRole("img", { name: "tracker" })).toBeNull();
    const docsLink = screen.getByRole("link", { name: "docs" });
    expect(docsLink.getAttribute("target")).toBe("_blank");
    expect(docsLink.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("publishes on click and shows the URL with a copy button", async () => {
    previewShareMock.mockResolvedValueOnce(SAMPLE_SNAPSHOT);
    createShareMock.mockResolvedValueOnce({
      id: "s-x",
      url: "https://gist.github.com/u/abc",
      created_at: "2026-05-21T12:00:00.000Z",
      snapshot_size_bytes: 200,
    });
    render(<ShareDialog open={true} onOpenChange={() => {}} taskId="t-1" sessionId="sess-1" />);

    const publishBtn = await screen.findByRole("button", { name: /Publish to GitHub Gist/i });
    fireEvent.click(publishBtn);

    await waitFor(() => expect(createShareMock).toHaveBeenCalledWith("t-1", "sess-1"));
    expect(await screen.findByText(/https:\/\/gist\.github\.com\/u\/abc/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Copy/i })).toBeTruthy();
  });

  it("shows the error state when preview fails and offers retry", async () => {
    previewShareMock.mockRejectedValueOnce(new Error("connect a GitHub account"));
    render(<ShareDialog open={true} onOpenChange={() => {}} taskId="t-1" sessionId="sess-1" />);

    expect(await screen.findByText(/connect a GitHub account/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Try again/i })).toBeTruthy();
  });

  it("renders an existing-shares row when listShares returns a share", async () => {
    previewShareMock.mockResolvedValueOnce(SAMPLE_SNAPSHOT);
    listSharesMock.mockResolvedValueOnce({
      shares: [
        {
          id: "s-old",
          url: "https://gist.github.com/u/old",
          created_at: "2026-05-20T12:00:00.000Z",
          snapshot_size_bytes: 100,
        },
      ],
    });
    render(<ShareDialog open={true} onOpenChange={() => {}} taskId="t-1" sessionId="sess-1" />);

    expect(await screen.findByText(/Active shares for this session/)).toBeTruthy();
    expect(screen.getByText(/https:\/\/gist\.github\.com\/u\/old/)).toBeTruthy();
  });
});
