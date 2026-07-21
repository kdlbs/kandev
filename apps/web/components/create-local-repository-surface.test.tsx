import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Repository } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  initialize: vi.fn(),
  listDirectory: vi.fn(),
  isMobile: false,
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  initializeLocalRepository: mocks.initialize,
}));

vi.mock("@/lib/api/domains/fs-api", () => ({
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
const CREATE_BUTTON_NAME = "Create repository";

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
  render(<CreateLocalRepositorySurface {...props} />);
  return props;
}

beforeEach(() => {
  mocks.isMobile = false;
  mocks.initialize.mockReset();
  mocks.listDirectory.mockReset();
  mocks.listDirectory.mockResolvedValue({ path: "/work", parent: "/", entries: [] });
});

afterEach(cleanup);

describe("local repository form helpers", () => {
  it.each(["", ".", "..", "nested/name", "nested\\name"])(
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
      <form onSubmit={parentSubmit}>
        <CreateLocalRepositorySurface {...props} />
      </form>,
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

  it("uses a mobile drawer instead of the desktop dialog on phones", async () => {
    mocks.isMobile = true;
    renderSurface();

    expect(await screen.findByTestId("create-local-repository-drawer")).toBeTruthy();
    expect(screen.queryByTestId("create-local-repository-dialog")).toBeNull();
  });
});
