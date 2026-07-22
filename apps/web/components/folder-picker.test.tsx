import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactElement, type ReactNode, cloneElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FolderPicker } from "./folder-picker";
import { listDirectory, type DirectoryListing } from "@/lib/api/domains/fs-api";

let setPopoverOpen: ((open: boolean) => void) | undefined;

vi.mock("@kandev/ui/popover", () => ({
  Popover: ({
    children,
    onOpenChange,
  }: {
    children: ReactNode;
    onOpenChange: (open: boolean) => void;
  }) => {
    setPopoverOpen = onOpenChange;
    return <div>{children}</div>;
  },
  PopoverTrigger: ({ children }: { children: ReactElement<{ onClick?: () => void }> }) =>
    cloneElement(children, { onClick: () => setPopoverOpen?.(true) }),
  PopoverContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/lib/api/domains/fs-api", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/lib/api/domains/fs-api")>();
  return { ...original, listDirectory: vi.fn() };
});

const mockedListDirectory = vi.mocked(listDirectory);
const BUTTON_ROLE = "button";
const TRIGGER_TEST_ID = "folder-picker-trigger";
const VIRTUAL_ROOT = "/";
const DRIVE_E_ROOT = "E:\\";
const SUCCESS_PATH = "E:\\Success";
const SUCCESS_NAME = "Success";

afterEach(() => {
  cleanup();
  mockedListDirectory.mockReset();
  setPopoverOpen = undefined;
});

describe("FolderPicker", () => {
  it("builds Windows breadcrumbs with a virtual root and native drive paths", async () => {
    mockedListDirectory
      .mockResolvedValueOnce(
        listing(VIRTUAL_ROOT, false, [{ name: DRIVE_E_ROOT, path: DRIVE_E_ROOT }]),
      )
      .mockResolvedValueOnce(listing("E:\\Projects\\Kandev", true))
      .mockResolvedValueOnce(listing(VIRTUAL_ROOT, false));

    render(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole(BUTTON_ROLE, { name: DRIVE_E_ROOT }));

    await screen.findByRole(BUTTON_ROLE, { name: "Kandev" });
    expect(screen.getByRole(BUTTON_ROLE, { name: VIRTUAL_ROOT })).toBeTruthy();
    expect(screen.getByRole(BUTTON_ROLE, { name: DRIVE_E_ROOT })).toBeTruthy();
    expect(screen.getByRole(BUTTON_ROLE, { name: "Projects" })).toBeTruthy();

    fireEvent.click(screen.getByRole(BUTTON_ROLE, { name: VIRTUAL_ROOT }));
    expect(mockedListDirectory).toHaveBeenLastCalledWith(VIRTUAL_ROOT);
  });

  it("clears a stale selectable listing when navigation fails", async () => {
    mockedListDirectory
      .mockResolvedValueOnce(listing("C:\\", true, [{ name: "Denied", path: "C:\\Denied" }]))
      .mockRejectedValueOnce(new Error("failed to list directory"));

    render(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole(BUTTON_ROLE, { name: "Denied" }));

    await screen.findByTestId("folder-picker-error");
    await waitFor(() =>
      expect(screen.getByTestId("folder-picker-choose").hasAttribute("disabled")).toBe(true),
    );
    expect(screen.queryByRole(BUTTON_ROLE, { name: "C:\\" })).toBeNull();
  });

  it("ignores an old success after a value change starts a successor load", async () => {
    const oldLoad = deferred<DirectoryListing>();
    const successorLoad = deferred<DirectoryListing>();
    mockedListDirectory
      .mockReturnValueOnce(oldLoad.promise)
      .mockReturnValueOnce(successorLoad.promise);

    const { rerender } = render(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await waitFor(() => expect(mockedListDirectory).toHaveBeenCalledWith(""));

    rerender(<FolderPicker value={SUCCESS_PATH} onChange={vi.fn()} />);
    await waitFor(() => expect(mockedListDirectory).toHaveBeenCalledWith(SUCCESS_PATH));
    await act(async () => successorLoad.resolve(listing(SUCCESS_PATH, true)));
    await waitFor(() =>
      expect(screen.getAllByRole(BUTTON_ROLE, { name: SUCCESS_NAME })).toHaveLength(2),
    );

    await act(async () => oldLoad.resolve(listing("C:\\Stale", true)));
    expect(screen.queryByRole(BUTTON_ROLE, { name: "Stale" })).toBeNull();
    expect(screen.getAllByRole(BUTTON_ROLE, { name: SUCCESS_NAME })).toHaveLength(2);
  });

  it("ignores an old error after close and reopen completes a successor load", async () => {
    const oldLoad = deferred<DirectoryListing>();
    const successorLoad = deferred<DirectoryListing>();
    mockedListDirectory
      .mockReturnValueOnce(oldLoad.promise)
      .mockReturnValueOnce(successorLoad.promise);

    render(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await waitFor(() => expect(mockedListDirectory).toHaveBeenCalledTimes(1));

    act(() => setPopoverOpen?.(false));
    act(() => setPopoverOpen?.(true));
    await waitFor(() => expect(mockedListDirectory).toHaveBeenCalledTimes(2));
    await act(async () => successorLoad.resolve(listing(SUCCESS_PATH, true)));
    await screen.findByRole(BUTTON_ROLE, { name: SUCCESS_NAME });

    await act(async () => oldLoad.reject(new Error("stale failure")));
    expect(screen.queryByTestId("folder-picker-error")).toBeNull();
    expect(screen.getByRole(BUTTON_ROLE, { name: SUCCESS_NAME })).toBeTruthy();
    expect(screen.getByTestId("folder-picker-choose").hasAttribute("disabled")).toBe(false);
  });
});

function listing(
  path: string,
  choosable: boolean,
  entries: DirectoryListing["entries"] = [],
): DirectoryListing {
  return { path, parent: "", entries, choosable };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}
