import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  featureEnabled: true,
  push: vi.fn(),
  dialogProps: null as DialogProps | null,
}));

type DialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialValues?: {
    title?: string;
    description?: string;
    noRepository?: boolean;
    preferLocalExecutor?: boolean;
  };
  lockedFields?: { workflow?: boolean; repository?: boolean };
  onSuccess?: (task: { id: string }, mode: "create" | "edit") => void;
};

const CREATE_CANVAS_LABEL = "Create canvas";
const SET_UP_CANVAS_LABEL = "Set up a canvas";
const CREATE_CANVAS_TITLE = "Create a canvas";
const CREATE_CANVAS_GOAL =
  "Create a new Kandev canvas with a coordinator view that lists the existing tasks.";
const CREATE_CANVAS_PROMPT = `${CREATE_CANVAS_GOAL}\n\n@create-canvas`;

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.featureEnabled,
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: (props: DialogProps) => {
    mocks.dialogProps = props;
    return props.open ? (
      <button
        type="button"
        data-testid="canvas-dialog-submit"
        onClick={() => props.onSuccess?.({ id: "task-1" }, "create")}
      >
        submit
      </button>
    ) : null;
  },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "canvases:createCanvas": CREATE_CANVAS_LABEL,
        "canvases:setUpCanvas": SET_UP_CANVAS_LABEL,
        "canvases:createCanvasTaskTitle": CREATE_CANVAS_TITLE,
        "canvases:createCanvasTaskPrompt": CREATE_CANVAS_GOAL,
      })[key] ?? key,
  }),
}));

import { CanvasTaskCreateLauncher } from "./canvas-task-create-launcher";

beforeEach(() => {
  mocks.featureEnabled = true;
  mocks.push.mockReset();
  mocks.dialogProps = null;
});

afterEach(cleanup);

describe("CanvasTaskCreateLauncher", () => {
  it("uses the localized scratch preset from the settings entry point", () => {
    render(<CanvasTaskCreateLauncher workspaceId="workspace-1" />);

    fireEvent.click(screen.getByRole("button", { name: CREATE_CANVAS_LABEL }));

    expect(mocks.dialogProps?.initialValues).toEqual({
      title: CREATE_CANVAS_TITLE,
      description: CREATE_CANVAS_PROMPT,
      noRepository: true,
      preferLocalExecutor: true,
    });
    expect(mocks.dialogProps?.lockedFields).toBeUndefined();

    fireEvent.click(screen.getByTestId("canvas-dialog-submit"));
    expect(mocks.push).toHaveBeenCalledWith("/t/task-1");
  });

  it("uses a semantic sidebar setup button and the same task preset", () => {
    render(<CanvasTaskCreateLauncher workspaceId="workspace-1" presentation="sidebar" />);

    const setup = screen.getByTestId("sidebar-canvases-empty");
    expect(setup.tagName).toBe("BUTTON");
    expect(setup.textContent).toContain(SET_UP_CANVAS_LABEL);
    expect(setup.getAttribute("href")).toBeNull();

    fireEvent.click(setup);

    expect(mocks.dialogProps?.initialValues).toEqual({
      title: CREATE_CANVAS_TITLE,
      description: CREATE_CANVAS_PROMPT,
      noRepository: true,
      preferLocalExecutor: true,
    });
  });

  it("closes the draft when its workspace or feature availability changes", () => {
    const { rerender } = render(<CanvasTaskCreateLauncher workspaceId="workspace-1" />);
    fireEvent.click(screen.getByRole("button", { name: CREATE_CANVAS_LABEL }));
    expect(mocks.dialogProps?.open).toBe(true);

    rerender(<CanvasTaskCreateLauncher workspaceId="workspace-2" />);
    expect(mocks.dialogProps?.open).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: CREATE_CANVAS_LABEL }));
    mocks.featureEnabled = false;
    rerender(<CanvasTaskCreateLauncher workspaceId="workspace-2" />);
    expect(screen.queryByRole("button", { name: CREATE_CANVAS_LABEL })).toBeNull();
  });

  it("does not expose a canvas action while the feature is disabled", () => {
    mocks.featureEnabled = false;

    render(<CanvasTaskCreateLauncher workspaceId="workspace-1" />);

    expect(screen.queryByRole("button", { name: CREATE_CANVAS_LABEL })).toBeNull();
    expect(mocks.dialogProps).toBeNull();
  });
});
