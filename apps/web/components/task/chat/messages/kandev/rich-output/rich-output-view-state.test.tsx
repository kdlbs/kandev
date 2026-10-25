import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { KandevRendererProps } from "../types";
import { RichOutputRenderer } from "./rich-output-renderer";
import type { AppState } from "@/lib/state/store";
const TOGGLE = "rich-output-file-preview-toggle";
const EXPANDED = "aria-expanded";
const owner = {};
const transport = vi.hoisted(() => ({
  read: vi.fn(),
  client: {},
  auth: "user-a",
  defaultExpanded: true,
}));
vi.mock("@/components/state-provider", () => ({
  useOptionalAppStoreApi: () => owner,
  useOptionalAppStore: (
    selector: (state: Pick<AppState, "auth" | "imagePreviewsExpandedByDefault">) => unknown,
  ) =>
    selector({
      auth: {
        mode: "disabled",
        authenticated: true,
        user: { id: transport.auth, email: "", display_name: "", role: "admin", status: "active" },
      },
      imagePreviewsExpandedByDefault: transport.defaultExpanded,
    }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => transport.client }));
vi.mock("@/lib/ws/workspace-files", () => ({ requestFileContent: transport.read }));
function Harness(props: KandevRendererProps) {
  return RichOutputRenderer(props);
}
afterEach(() => {
  cleanup();
  transport.read.mockReset();
  transport.auth = "user-a";
  transport.defaultExpanded = true;
});

// @covers AC-AGENTS-IMAGE-PREVIEW-DEFAULTS-001.3
it("retains hidden image choices across metadata changes and layout remounts independently", async () => {
  const leaf = { type: "FilePreview", props: { path: "shared.png" }, children: [] };
  const props: KandevRendererProps = {
    sessionId: "image-session",
    messageId: "image-message",
    status: "complete",
    result: undefined,
    args: {
      version: 2,
      catalog_version: 1,
      title: "Images",
      spec: {
        root: "report",
        elements: {
          report: {
            type: "Stack",
            props: { direction: "vertical" },
            children: ["first", "second"],
          },
          first: structuredClone(leaf),
          second: structuredClone(leaf),
        },
      },
    },
  };
  transport.read.mockResolvedValue({ content: "aW1hZ2U=", is_binary: true });
  const view = render(<Harness {...props} />);
  expect(await screen.findAllByRole("button", { name: "Open shared.png" })).toHaveLength(2);
  fireEvent.click(screen.getAllByTestId(TOGGLE)[0]);
  const renamed = { ...props, args: { ...props.args, title: "Renamed images" } };
  view.rerender(<Harness {...renamed} />);
  expect(screen.getAllByTestId(TOGGLE).map((toggle) => toggle.getAttribute(EXPANDED))).toEqual([
    "false",
    "true",
  ]);
  expect(transport.read).toHaveBeenCalledTimes(2);
  view.unmount();
  render(<Harness {...renamed} />);
  expect(screen.getAllByTestId(TOGGLE).map((toggle) => toggle.getAttribute(EXPANDED))).toEqual([
    "false",
    "true",
  ]);
  expect(await screen.findAllByRole("button", { name: "Open shared.png" })).toHaveLength(1);
  expect(transport.read).toHaveBeenCalledTimes(3);
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.9
it("restores disclosure preferences across layout remounts without retaining file bytes", async () => {
  const props: KandevRendererProps = {
    sessionId: "session",
    messageId: "message",
    status: "complete",
    result: undefined,
    args: {
      version: 2,
      catalog_version: 1,
      title: "Report",
      spec: {
        root: "file",
        elements: { file: { type: "FilePreview", props: { path: "notes.txt" }, children: [] } },
      },
    },
  };
  transport.read.mockResolvedValueOnce({ content: "Old file bytes", is_binary: false });
  const view = render(<Harness {...props} />);
  fireEvent.click(screen.getByTestId(TOGGLE));
  await screen.findByText("Old file bytes");
  view.unmount();
  transport.read.mockResolvedValueOnce({ content: "Fresh file bytes", is_binary: false });
  const restored = render(<Harness {...props} />);
  expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("true");
  await screen.findByText("Fresh file bytes");
  expect(transport.read).toHaveBeenCalledTimes(2);
  restored.rerender(<Harness {...props} messageId="another-message" />);
  expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("false");
  expect(screen.queryByText("Fresh file bytes")).toBeNull();
});

it("keeps identical file leaves independent by element identity", async () => {
  const leaf = { type: "FilePreview", props: { path: "same.txt" }, children: [] };
  transport.read.mockResolvedValue({ content: "Shared source", is_binary: false });
  render(
    <Harness
      sessionId="s2"
      messageId="m2"
      status="complete"
      result={undefined}
      args={{
        version: 2,
        catalog_version: 1,
        title: "Files",
        spec: {
          root: "report",
          elements: {
            report: {
              type: "Stack",
              props: { direction: "vertical" },
              children: ["first", "second"],
            },
            first: structuredClone(leaf),
            second: structuredClone(leaf),
          },
        },
      }}
    />,
  );
  const [first, second] = screen.getAllByTestId(TOGGLE);
  fireEvent.click(first);
  await screen.findByText("Shared source");
  expect(first.getAttribute(EXPANDED)).toBe("true");
  expect(second.getAttribute(EXPANDED)).toBe("false");
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.9
it("retains standalone disclosure on rerender and clears it across authentication", async () => {
  const props: KandevRendererProps = {
    sessionId: "standalone-session",
    status: "complete",
    result: undefined,
    args: {
      version: 2,
      catalog_version: 1,
      title: "Standalone report",
      spec: {
        root: "file",
        elements: { file: { type: "FilePreview", props: { path: "notes.txt" }, children: [] } },
      },
    },
  };
  transport.read.mockResolvedValueOnce({ content: "Standalone bytes", is_binary: false });
  const view = render(<Harness {...props} />);
  fireEvent.click(screen.getByTestId(TOGGLE));
  await screen.findByText("Standalone bytes");
  const renamed = { ...props, args: { ...props.args, title: "Renamed report" } };
  view.rerender(<Harness {...renamed} />);
  expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("true");
  expect(transport.read).toHaveBeenCalledTimes(1);
  transport.auth = "user-b";
  view.rerender(<Harness {...renamed} />);
  expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("false");
  expect(screen.queryByText("Standalone bytes")).toBeNull();
  expect(transport.read).toHaveBeenCalledTimes(1);
});

// @covers AC-AGENTS-IMAGE-PREVIEW-EXPANSION-001.6
it.each([1, 2])(
  "starts version %i images collapsed without reads, but keeps manual opening and enlargement",
  async (version) => {
    transport.defaultExpanded = false;
    transport.read.mockResolvedValue({ content: "aW1hZ2U=", is_binary: true });
    const props: KandevRendererProps = {
      sessionId: `default-session-${version}`,
      messageId: `default-message-${version}`,
      status: "complete",
      result: undefined,
      args:
        version === 1
          ? { version: 1, title: "Images", blocks: [{ type: "file", path: "manual.png" }] }
          : {
              version: 2,
              catalog_version: 1,
              title: "Images",
              spec: {
                root: "image",
                elements: {
                  image: { type: "FilePreview", props: { path: "manual.png" }, children: [] },
                },
              },
            },
    };
    const view = render(<Harness {...props} />);
    expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("false");
    expect(transport.read).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Open manual.png" })).toBeNull();
    fireEvent.click(screen.getByTestId(TOGGLE));
    const trigger = await screen.findByRole("button", { name: "Open manual.png" });
    await act(async () => fireEvent.click(trigger));
    expect(screen.getByRole("dialog", { name: "Image preview" })).toBeTruthy();
    expect(transport.read).toHaveBeenCalledTimes(1);
    view.unmount();
    const restored = render(<Harness {...props} />);
    expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("true");
    await screen.findByRole("button", { name: "Open manual.png" });
    fireEvent.click(screen.getByTestId(TOGGLE));
    restored.unmount();
    transport.defaultExpanded = true;
    render(<Harness {...props} />);
    expect(screen.getByTestId(TOGGLE).getAttribute(EXPANDED)).toBe("false");
    expect(transport.read).toHaveBeenCalledTimes(2);
  },
);
