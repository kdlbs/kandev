import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { DirectoryListing } from "@/lib/api/domains/fs-api";
import type { Repository } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  initialize: vi.fn(),
  createDirectory: vi.fn(),
  listDirectory: vi.fn(),
  isMobile: false,
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  initializeLocalRepository: mocks.initialize,
}));

vi.mock("@/lib/api/domains/fs-api", () => ({
  createDirectory: mocks.createDirectory,
  listDirectory: mocks.listDirectory,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));

import {
  buildLocalRepositoryTargetPath,
  CreateLocalRepositorySurface,
  validateLocalRepositoryName,
} from "./create-local-repository-surface";

const REPOSITORY_NAME = "alpha";
const REPOSITORY_NAME_LABEL = "Repository name";
const PARENT_DIRECTORY_LABEL = "Parent directory";
const CREATE_BUTTON_NAME = "Create repository";
const NEW_FOLDER_BUTTON_NAME = "New folder";
const NEW_FOLDER_NAME_LABEL = "New folder name";
const CREATE_FOLDER_BUTTON_NAME = "Create folder";
const PROJECTS_PATH = "/work/projects";

const createdRepository = {
  id: "repo-new",
  workspace_id: "ws-1",
  name: REPOSITORY_NAME,
  source_type: "local",
  local_path: "/work/alpha",
  default_branch: "main",
  created_at: "",
  updated_at: "",
} as Repository;

const directLocalSelection = {
  executorId: "local",
  executorProfileId: "local-profile",
  executorProfileName: "This computer",
  requiresSwitch: true,
};

function renderSurface(
  overrides: Partial<React.ComponentProps<typeof CreateLocalRepositorySurface>> = {},
) {
  const props: React.ComponentProps<typeof CreateLocalRepositorySurface> = {
    open: true,
    onOpenChange: vi.fn(),
    workspaceId: "ws-1",
    executorSelection: directLocalSelection,
    onCreated: vi.fn(),
    ...overrides,
  };
  render(
    <StateProvider>
      <CreateLocalRepositorySurface {...props} />
    </StateProvider>,
  );
  return props;
}

beforeEach(() => {
  mocks.isMobile = false;
  mocks.initialize.mockReset();
  mocks.createDirectory.mockReset();
  mocks.listDirectory.mockReset();
  mocks.listDirectory.mockResolvedValue({ path: "/work", parent: "/", entries: [] });
});

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

describe("local repository form helpers", () => {
  it.each(["", ".", "..", "nested/name", "nested\\name", "name\0with-null"])(
    "rejects invalid repository name %j",
    (name) => {
      expect(validateLocalRepositoryName(name)).not.toBeNull();
    },
  );

  it("trims a valid name and derives its target path", () => {
    expect(validateLocalRepositoryName(` ${REPOSITORY_NAME} `)).toBeNull();
    expect(buildLocalRepositoryTargetPath("/work/projects/", ` ${REPOSITORY_NAME} `)).toBe(
      "/work/projects/alpha",
    );
  });
});

describe("CreateLocalRepositorySurface", () => {
  it("shows the target and initializes the repository immediately", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    const props = renderSurface();

    const nameInput = await screen.findByLabelText(REPOSITORY_NAME_LABEL);
    fireEvent.change(nameInput, { target: { value: REPOSITORY_NAME } });
    expect(screen.getByText("/work/alpha")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));

    await waitFor(() => {
      expect(mocks.initialize).toHaveBeenCalledWith("ws-1", {
        name: REPOSITORY_NAME,
        parentPath: "/work",
      });
    });
    expect(props.onCreated).toHaveBeenCalledWith(createdRepository);
  });

  it("submits a manually entered parent directory without browsing to it", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    renderSurface();

    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    fireEvent.change(screen.getByLabelText(PARENT_DIRECTORY_LABEL), {
      target: { value: "/work/manual-parent" },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));

    await waitFor(() => {
      expect(mocks.initialize).toHaveBeenCalledWith("ws-1", {
        name: REPOSITORY_NAME,
        parentPath: "/work/manual-parent",
      });
    });
  });

  it("updates the parent directory input when browsing folders", async () => {
    mocks.listDirectory.mockImplementation(async (path: string) => {
      if (path === "") {
        return {
          path: "/work",
          parent: "/",
          entries: [{ name: "projects", path: PROJECTS_PATH }],
        };
      }
      return { path, parent: "/work", entries: [] };
    });
    renderSurface();

    fireEvent.click(await screen.findByTestId("folder-picker-entry"));

    await waitFor(() => {
      expect((screen.getByLabelText(PARENT_DIRECTORY_LABEL) as HTMLInputElement).value).toBe(
        PROJECTS_PATH,
      );
    });
  });

  it("creates a folder from the navigator and enters it", async () => {
    mocks.createDirectory.mockResolvedValue({ path: PROJECTS_PATH });
    mocks.listDirectory.mockImplementation(async (path: string) => ({
      path: path || "/work",
      parent: "/",
      entries: [],
      choosable: true,
    }));
    renderSurface();

    await waitFor(() =>
      expect(
        (screen.getByRole("button", { name: NEW_FOLDER_BUTTON_NAME }) as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
    fireEvent.click(screen.getByRole("button", { name: NEW_FOLDER_BUTTON_NAME }));
    fireEvent.change(screen.getByRole("textbox", { name: NEW_FOLDER_NAME_LABEL }), {
      target: { value: "projects" },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_FOLDER_BUTTON_NAME }));

    await waitFor(() => {
      expect(mocks.createDirectory).toHaveBeenCalledWith("/work", "projects");
      expect((screen.getByLabelText(PARENT_DIRECTORY_LABEL) as HTMLInputElement).value).toBe(
        PROJECTS_PATH,
      );
    });
  });
  // @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.4
  it("exposes the shared reveal control and re-lists this browser with it", async () => {
    const hiddenListing = {
      path: "/work",
      parent: "/",
      entries: [{ name: ".config", path: "/work/.config" }],
      choosable: true,
    };
    mocks.listDirectory
      .mockResolvedValueOnce({ ...hiddenListing, entries: [] })
      .mockResolvedValue(hiddenListing);
    renderSurface();

    const control = await screen.findByRole("switch", { name: "Hidden folders" });
    expect(control.getAttribute("aria-checked")).toBe("false");

    fireEvent.click(control);

    // This browser asks the same shared preference the other directory browsers
    // read, so a toggle anywhere in the application reaches this surface too.
    await waitFor(() =>
      expect(mocks.listDirectory).toHaveBeenLastCalledWith("", { includeHidden: true }),
    );
    await waitFor(() => expect(screen.getByText(".config")).toBeTruthy());
    expect(await screen.findByRole("switch", { name: "Hidden folders" })).toBeTruthy();
  });
});

describe("CreateLocalRepositorySurface visibility refresh", () => {
  it("preserves an unfinished folder name during a visibility refresh", async () => {
    let finishRefresh!: (listing: DirectoryListing) => void;
    const refresh = new Promise<DirectoryListing>((resolve) => {
      finishRefresh = resolve;
    });
    const currentListing: DirectoryListing = {
      path: "/work",
      parent: "/",
      entries: [],
      choosable: true,
    };
    mocks.listDirectory.mockResolvedValue(currentListing);
    renderSurface();

    await waitFor(() =>
      expect(
        (screen.getByRole("button", { name: NEW_FOLDER_BUTTON_NAME }) as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
    fireEvent.click(screen.getByRole("button", { name: NEW_FOLDER_BUTTON_NAME }));
    const nameInput = screen.getByRole("textbox", { name: NEW_FOLDER_NAME_LABEL });
    fireEvent.change(nameInput, { target: { value: "unfinished-folder" } });
    mocks.listDirectory.mockImplementation(
      (_path: string, options?: { includeHidden?: boolean }) =>
        options?.includeHidden ? refresh : Promise.resolve(currentListing),
    );
    fireEvent.click(await screen.findByRole("switch", { name: "Hidden folders" }));

    await waitFor(() =>
      expect(mocks.listDirectory).toHaveBeenCalledWith("", { includeHidden: true }),
    );
    expect(
      (screen.getByRole("textbox", { name: NEW_FOLDER_NAME_LABEL }) as HTMLInputElement).value,
    ).toBe("unfinished-folder");

    await act(async () =>
      finishRefresh({
        ...currentListing,
        entries: [{ name: ".config", path: "/work/.config" }],
      }),
    );
    expect(
      (screen.getByRole("textbox", { name: NEW_FOLDER_NAME_LABEL }) as HTMLInputElement).value,
    ).toBe("unfinished-folder");
  });
});

describe("CreateLocalRepositorySurface async creation", () => {
  it("delivers a delayed response to the current row handler", async () => {
    let complete!: (repository: Repository) => void;
    mocks.initialize.mockReturnValue(
      new Promise<Repository>((resolve) => {
        complete = resolve;
      }),
    );
    const previousHandler = vi.fn();
    const currentHandler = vi.fn();
    const props = {
      open: true,
      onOpenChange: vi.fn(),
      workspaceId: "ws-1",
      executorSelection: directLocalSelection,
      onCreated: previousHandler,
    };
    const { rerender } = render(
      <StateProvider>
        <CreateLocalRepositorySurface {...props} />
      </StateProvider>,
    );
    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    await waitFor(() => expect(mocks.initialize).toHaveBeenCalledOnce());
    rerender(
      <StateProvider>
        <CreateLocalRepositorySurface {...props} onCreated={currentHandler} />
      </StateProvider>,
    );
    complete(createdRepository);
    await waitFor(() => expect(previousHandler).toHaveBeenCalledWith(createdRepository));
    expect(currentHandler).not.toHaveBeenCalled();
  });

  it("creates for a multi-row task without a direct-local profile", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    const props = renderSurface({ context: "task-create-multi", executorSelection: null });
    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    await waitFor(() => expect(props.onCreated).toHaveBeenCalledWith(createdRepository));
  });

  it("keeps the surface open when the completion is no longer current", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    const onOpenChange = vi.fn();
    const onCreated = vi.fn(() => false);
    renderSurface({ onOpenChange, onCreated });
    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(createdRepository));
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});

describe("CreateLocalRepositorySurface submission", () => {
  it("does not submit the parent task form", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    const parentSubmit = vi.fn((event: React.FormEvent) => event.preventDefault());
    const props: React.ComponentProps<typeof CreateLocalRepositorySurface> = {
      open: true,
      onOpenChange: vi.fn(),
      workspaceId: "ws-1",
      executorSelection: directLocalSelection,
      onCreated: vi.fn(),
    };

    render(
      <StateProvider>
        <form onSubmit={parentSubmit}>
          <CreateLocalRepositorySurface {...props} />
        </form>
      </StateProvider>,
    );

    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));

    await waitFor(() => expect(props.onCreated).toHaveBeenCalledWith(createdRepository));
    expect(parentSubmit).not.toHaveBeenCalled();
  });

  it("retains entered values and allows retry after a conflict", async () => {
    mocks.initialize
      .mockRejectedValueOnce(new Error("A file or folder already exists at that path"))
      .mockResolvedValueOnce(createdRepository);
    renderSurface();

    const nameInput = await screen.findByLabelText(REPOSITORY_NAME_LABEL);
    fireEvent.change(nameInput, { target: { value: REPOSITORY_NAME } });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    expect((await screen.findByRole("alert")).textContent).toContain("already exists");
    expect((nameInput as HTMLInputElement).value).toBe(REPOSITORY_NAME);

    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    await waitFor(() => expect(mocks.initialize).toHaveBeenCalledTimes(2));
  });

  it("clears a submission error when the user corrects the form", async () => {
    mocks.initialize.mockRejectedValue(new Error("parent directory cannot be accessed"));
    renderSurface();

    const nameInput = await screen.findByLabelText(REPOSITORY_NAME_LABEL);
    fireEvent.change(nameInput, { target: { value: REPOSITORY_NAME } });
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));
    expect(await screen.findByRole("alert")).toBeTruthy();

    fireEvent.change(screen.getByLabelText(PARENT_DIRECTORY_LABEL), {
      target: { value: "/work/new-parent" },
    });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("blocks initialization when no direct-local profile exists", async () => {
    renderSurface({ executorSelection: null });

    const nameInput = await screen.findByLabelText(REPOSITORY_NAME_LABEL);
    fireEvent.change(nameInput, { target: { value: REPOSITORY_NAME } });

    expect(screen.getByText(/direct local executor profile is required/i)).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: CREATE_BUTTON_NAME }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(mocks.initialize).not.toHaveBeenCalled();
  });

  it("allows workspace repository creation without changing an executor", async () => {
    mocks.initialize.mockResolvedValue(createdRepository);
    const props = renderSurface({ context: "workspace", executorSelection: null });

    fireEvent.change(await screen.findByLabelText(REPOSITORY_NAME_LABEL), {
      target: { value: REPOSITORY_NAME },
    });
    expect(screen.getByText(/registers it in this workspace/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: CREATE_BUTTON_NAME }));

    await waitFor(() => expect(props.onCreated).toHaveBeenCalledWith(createdRepository));
  });

  it("uses a mobile drawer instead of the desktop dialog on phones", async () => {
    mocks.isMobile = true;
    renderSurface();

    expect(await screen.findByTestId("create-local-repository-drawer")).toBeTruthy();
    expect(screen.queryByTestId("create-local-repository-dialog")).toBeNull();
  });
});
