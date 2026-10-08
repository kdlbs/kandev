import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactElement, type ReactNode, cloneElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FolderPicker } from "./folder-picker";
import { StateProvider } from "@/components/state-provider";
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
const SHOW_HIDDEN_TEST_ID = "directory-browser-show-hidden";
const CHOOSE_BUTTON_TEST_ID = "folder-picker-choose";
const SWITCH_ROLE = "switch";
const HIDDEN_FOLDERS_NAME = "Hidden folders";
const HOME_USER = "example";
const HOME_BREADCRUMB_NAME = HOME_USER;
const PROJECTS_NAME = "projects";
const VIRTUAL_ROOT = "/";
const DRIVE_E_ROOT = "E:\\";
const SUCCESS_PATH = "E:\\Success";
const SUCCESS_NAME = "Success";
const HOME = `/home/${HOME_USER}`;
const SELECTED_PATH = `/Users/${HOME_USER}/Code`;

afterEach(() => {
  cleanup();
  mockedListDirectory.mockReset();
  setPopoverOpen = undefined;
  window.localStorage.clear();
  delete (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
  delete (window as Window & { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__;
});

function renderPicker(node: ReactElement) {
  return render(<StateProvider>{node}</StateProvider>);
}

describe("FolderPicker hidden-entry reveal", () => {
  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.1, AC-WORKSPACES-HIDDEN-FOLDERS-001.2
  it("offers the reveal control inactive and asks for the current listing first", async () => {
    mockedListDirectory.mockResolvedValue(
      listing(HOME, true, [{ name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` }]),
    );

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await screen.findByTestId(SHOW_HIDDEN_TEST_ID);

    expect(mockedListDirectory).toHaveBeenCalledWith("", { includeHidden: false });
    // A switch names the thing it controls and reports its own state, so the
    // accessible name is one short localized noun that never changes. Looking it
    // up by role is also what proves the copy went through translation.
    const control = await screen.findByRole(SWITCH_ROLE, { name: HIDDEN_FOLDERS_NAME });
    expect(control.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByRole(BUTTON_ROLE, { name: ".minimax" })).toBeNull();
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.3
  it("re-lists the same path with the reveal on and shows the hidden entry", async () => {
    mockedListDirectory
      .mockResolvedValueOnce(
        listing(HOME, true, [{ name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` }]),
      )
      .mockResolvedValueOnce(
        listing(HOME, true, [
          { name: ".minimax", path: `${HOME}/.minimax` },
          { name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` },
        ]),
      );

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByTestId(SHOW_HIDDEN_TEST_ID));

    await screen.findByRole(BUTTON_ROLE, { name: ".minimax" });
    // The reveal must re-list the directory the user is already in, not jump
    // anywhere: the breadcrumb and the selection target stay put.
    expect(mockedListDirectory).toHaveBeenLastCalledWith("", { includeHidden: true });
    const control = await screen.findByRole(SWITCH_ROLE, { name: HIDDEN_FOLDERS_NAME });
    expect(control.getAttribute("aria-checked")).toBe("true");
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.3
  it("re-lists the directory being browsed instead of snapping back to the chosen one", async () => {
    // Browsing does not commit a value, so the toggle must re-list wherever the
    // user currently is. Falling back to the chosen path would throw away their
    // position for a display-only preference.
    const childPath = `${HOME}/${PROJECTS_NAME}`;
    const childEntries = [{ name: ".minimax", path: `${childPath}/.minimax` }];
    mockedListDirectory
      .mockResolvedValueOnce(listing(HOME, true, [{ name: PROJECTS_NAME, path: childPath }]))
      .mockResolvedValueOnce(listing(childPath, true, childEntries))
      .mockResolvedValue(listing(childPath, true, childEntries));

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole(BUTTON_ROLE, { name: PROJECTS_NAME }));
    await waitFor(() =>
      expect(mockedListDirectory).toHaveBeenLastCalledWith(childPath, { includeHidden: false }),
    );

    fireEvent.click(await screen.findByTestId(SHOW_HIDDEN_TEST_ID));

    await screen.findByRole(BUTTON_ROLE, { name: ".minimax" });
    expect(mockedListDirectory).toHaveBeenLastCalledWith(childPath, { includeHidden: true });
    // The user is still inside the directory they descended into.
    expect(await screen.findByRole(BUTTON_ROLE, { name: PROJECTS_NAME })).toBeTruthy();
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.5
  it("persists the choice so the next browser session reopens with the reveal on", async () => {
    mockedListDirectory.mockResolvedValue(listing(HOME, true));

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByTestId(SHOW_HIDDEN_TEST_ID));

    // Reading a stored preference back is the slice's own contract, covered in
    // ui-slice.test.ts against a real page load. This asserts the control is the
    // thing that writes it.
    await waitFor(() =>
      expect(window.localStorage.getItem("kandev.directoryBrowser.showHidden")).toBe("true"),
    );
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.9
  it("keeps the control reachable as a switch with a stable localized name", async () => {
    mockedListDirectory.mockResolvedValue(listing(HOME, true));

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const control = await screen.findByRole(SWITCH_ROLE, { name: HIDDEN_FOLDERS_NAME });
    expect(control.tagName).toBe("BUTTON");
    expect(control.hasAttribute("disabled")).toBe(false);
    expect(control.getAttribute("aria-checked")).toBe("false");
  });

  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.12
  it("renders no reveal control in the Tauri WebView", async () => {
    const invoke = vi.fn(async () => ({ status: "cancelled" as const }));
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke },
    });
    Object.defineProperty(window, "__KANDEV_BOOT_PAYLOAD__", {
      configurable: true,
      value: { runtime: { nativeFolderPickerAvailable: true } },
    });

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    await waitFor(() => expect(invoke).toHaveBeenCalledWith("pick_directory", undefined));
    expect(screen.queryByTestId(SHOW_HIDDEN_TEST_ID)).toBeNull();
    expect(mockedListDirectory).not.toHaveBeenCalled();
  });
});

describe("FolderPicker visibility refresh", () => {
  it("keeps the current path selectable while a visibility refresh is pending", async () => {
    const refresh = deferred<DirectoryListing>();
    mockedListDirectory.mockResolvedValue(
      listing(HOME, true, [{ name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` }]),
    );

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await screen.findByRole(BUTTON_ROLE, { name: PROJECTS_NAME });
    mockedListDirectory.mockImplementation((_path, options) =>
      options?.includeHidden ? refresh.promise : Promise.resolve(listing(HOME, true)),
    );

    fireEvent.click(await screen.findByTestId(SHOW_HIDDEN_TEST_ID));
    await waitFor(() =>
      expect(mockedListDirectory).toHaveBeenCalledWith("", { includeHidden: true }),
    );

    expect(
      screen.getByRole(BUTTON_ROLE, { name: new RegExp(`^${HOME_BREADCRUMB_NAME}$`) }),
    ).toBeTruthy();
    expect(screen.getByTestId(CHOOSE_BUTTON_TEST_ID).hasAttribute("disabled")).toBe(false);

    await act(async () => refresh.resolve(listing(HOME, true)));
    expect(screen.getByTestId(CHOOSE_BUTTON_TEST_ID).hasAttribute("disabled")).toBe(false);
  });

  it("keeps the current path selectable when a visibility refresh fails", async () => {
    mockedListDirectory.mockResolvedValue(
      listing(HOME, true, [{ name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` }]),
    );

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await screen.findByRole(BUTTON_ROLE, { name: PROJECTS_NAME });
    mockedListDirectory.mockImplementation((_path, options) =>
      options?.includeHidden
        ? Promise.reject(new Error("refresh failed"))
        : Promise.resolve(
            listing(HOME, true, [{ name: PROJECTS_NAME, path: `${HOME}/${PROJECTS_NAME}` }]),
          ),
    );

    fireEvent.click(await screen.findByTestId(SHOW_HIDDEN_TEST_ID));
    await waitFor(() =>
      expect(mockedListDirectory).toHaveBeenCalledWith("", { includeHidden: true }),
    );

    await screen.findByTestId("folder-picker-error");
    expect(
      screen.getByRole(BUTTON_ROLE, { name: new RegExp(`^${HOME_BREADCRUMB_NAME}$`) }),
    ).toBeTruthy();
    expect(screen.getByTestId(CHOOSE_BUTTON_TEST_ID).hasAttribute("disabled")).toBe(false);
  });
});

describe("FolderPicker trigger", () => {
  it("preserves the separator when displaying the POSIX root", () => {
    renderPicker(<FolderPicker value="/" onChange={vi.fn()} placeholder="Pick a folder" />);

    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toBe("/");
  });

  it("preserves the separator when displaying a Windows drive root", () => {
    renderPicker(<FolderPicker value="C:\\" onChange={vi.fn()} />);

    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toBe("C:\\");
  });

  it("uses the native picker in a Tauri WebView without listing directories over HTTP", async () => {
    const invoke = vi.fn(async () => ({ status: "selected", path: SELECTED_PATH }));
    Object.defineProperty(window, "__TAURI_INTERNALS__", {
      configurable: true,
      value: { invoke },
    });
    Object.defineProperty(window, "__KANDEV_BOOT_PAYLOAD__", {
      configurable: true,
      value: { runtime: { nativeFolderPickerAvailable: true } },
    });
    const onChange = vi.fn();

    renderPicker(<FolderPicker value="" onChange={onChange} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    await waitFor(() => expect(onChange).toHaveBeenCalledWith(SELECTED_PATH));
    expect(invoke).toHaveBeenCalledWith("pick_directory", undefined);
    expect(mockedListDirectory).not.toHaveBeenCalled();
    expect(screen.queryByTestId("folder-picker-popover")).toBeNull();
  });
});

describe("FolderPicker navigation", () => {
  it("builds Windows breadcrumbs with a virtual root and native drive paths", async () => {
    mockedListDirectory
      .mockResolvedValueOnce(
        listing(VIRTUAL_ROOT, false, [{ name: DRIVE_E_ROOT, path: DRIVE_E_ROOT }]),
      )
      .mockResolvedValueOnce(listing("E:\\Projects\\Kandev", true))
      .mockResolvedValueOnce(listing(VIRTUAL_ROOT, false));

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole(BUTTON_ROLE, { name: DRIVE_E_ROOT }));

    await screen.findByRole(BUTTON_ROLE, { name: "Kandev" });
    expect(screen.getByRole(BUTTON_ROLE, { name: VIRTUAL_ROOT })).toBeTruthy();
    expect(screen.getByRole(BUTTON_ROLE, { name: DRIVE_E_ROOT })).toBeTruthy();
    expect(screen.getByRole(BUTTON_ROLE, { name: "Projects" })).toBeTruthy();

    fireEvent.click(screen.getByRole(BUTTON_ROLE, { name: VIRTUAL_ROOT }));
    expect(mockedListDirectory).toHaveBeenLastCalledWith(VIRTUAL_ROOT, { includeHidden: false });
  });

  it("clears a stale selectable listing when navigation fails", async () => {
    mockedListDirectory
      .mockResolvedValueOnce(listing("C:\\", true, [{ name: "Denied", path: "C:\\Denied" }]))
      .mockRejectedValueOnce(new Error("failed to list directory"));

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    fireEvent.click(await screen.findByRole(BUTTON_ROLE, { name: "Denied" }));

    await screen.findByTestId("folder-picker-error");
    await waitFor(() =>
      expect(screen.getByTestId(CHOOSE_BUTTON_TEST_ID).hasAttribute("disabled")).toBe(true),
    );
    expect(screen.queryByRole(BUTTON_ROLE, { name: "C:\\" })).toBeNull();
  });

  it("ignores an old success after a value change starts a successor load", async () => {
    const oldLoad = deferred<DirectoryListing>();
    const successorLoad = deferred<DirectoryListing>();
    mockedListDirectory
      .mockReturnValueOnce(oldLoad.promise)
      .mockReturnValueOnce(successorLoad.promise);

    const { rerender } = renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
    await waitFor(() =>
      expect(mockedListDirectory).toHaveBeenCalledWith("", { includeHidden: false }),
    );

    rerender(
      <StateProvider>
        <FolderPicker value={SUCCESS_PATH} onChange={vi.fn()} />
      </StateProvider>,
    );
    await waitFor(() =>
      expect(mockedListDirectory).toHaveBeenCalledWith(SUCCESS_PATH, { includeHidden: false }),
    );
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

    renderPicker(<FolderPicker value="" onChange={vi.fn()} />);
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
    expect(screen.getByTestId(CHOOSE_BUTTON_TEST_ID).hasAttribute("disabled")).toBe(false);
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
