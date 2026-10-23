import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MobileFileActions } from "./mobile-file-actions";

afterEach(cleanup);
const MODE_TRIGGER_TEST_ID = "mobile-markdown-mode-trigger";
const SOURCE_MODE_TEST_ID = "mobile-markdown-mode-source";

const defaults = {
  path: "README.md",
  mode: "preview" as const,
  hybridAvailable: true,
  isDirty: true,
  isSaving: false,
  hasRemoteUpdate: true,
  onModeChange: vi.fn(),
  onSave: vi.fn(),
  onReloadFromAgent: vi.fn(),
};

it("keeps Save independent of the mode and reload menus", async () => {
  const onSave = vi.fn();
  const onReloadFromAgent = vi.fn();
  render(<MobileFileActions {...defaults} onSave={onSave} onReloadFromAgent={onReloadFromAgent} />);
  fireEvent.click(screen.getByTestId("mobile-file-save"));
  expect(onSave).toHaveBeenCalledOnce();
  expect(screen.queryByTestId("mobile-file-reload")).toBeNull();
  fireEvent.keyDown(screen.getByTestId("mobile-file-actions-trigger"), { key: "Enter" });
  const reload = await screen.findByTestId("mobile-file-reload");
  fireEvent.click(reload);
  expect(onReloadFromAgent).toHaveBeenCalledOnce();
});

it("selects a mode and disables unavailable Edit with an explanation", async () => {
  const onModeChange = vi.fn();
  render(<MobileFileActions {...defaults} hybridAvailable={false} onModeChange={onModeChange} />);
  fireEvent.keyDown(screen.getByTestId(MODE_TRIGGER_TEST_ID), { key: "Enter" });
  const edit = await screen.findByTestId("mobile-markdown-mode-edit");
  expect(edit.getAttribute("aria-disabled")).toBe("true");
  expect(edit.getAttribute("title")).toBeTruthy();
  fireEvent.click(screen.getByTestId(SOURCE_MODE_TEST_ID));
  expect(onModeChange).toHaveBeenCalledWith("source");
  await waitFor(() => expect(screen.queryByTestId(SOURCE_MODE_TEST_ID)).toBeNull());
});

it("omits Edit for MDX and disables Save while clean or saving", async () => {
  const view = render(<MobileFileActions {...defaults} path="README.mdx" isDirty={false} />);
  expect((screen.getByTestId("mobile-file-save") as HTMLButtonElement).disabled).toBe(true);
  fireEvent.keyDown(screen.getByTestId(MODE_TRIGGER_TEST_ID), { key: "Enter" });
  await screen.findByTestId(SOURCE_MODE_TEST_ID);
  expect(screen.queryByTestId("mobile-markdown-mode-edit")).toBeNull();
  view.rerender(<MobileFileActions {...defaults} isSaving />);
  expect((screen.getByTestId("mobile-file-save") as HTMLButtonElement).disabled).toBe(true);
});

it("records an explicit Source choice even when fallback already shows Source", async () => {
  const onModeChange = vi.fn();
  render(<MobileFileActions {...defaults} mode="source" onModeChange={onModeChange} />);
  fireEvent.keyDown(screen.getByTestId(MODE_TRIGGER_TEST_ID), { key: "Enter" });
  fireEvent.click(await screen.findByTestId(SOURCE_MODE_TEST_ID));
  expect(onModeChange).toHaveBeenCalledWith("source");
});
