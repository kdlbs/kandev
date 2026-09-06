import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  featureEnabled: true,
  push: vi.fn(),
  dialogProps: null as DialogProps | null,
}));

type DialogProps = {
  open: boolean;
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
const CREATE_CANVAS_TITLE = "Create a canvas";
const CREATE_CANVAS_PROMPT = "Build a canvas application";

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
        "canvases:createCanvasTaskTitle": CREATE_CANVAS_TITLE,
        "canvases:createCanvasTaskPrompt": CREATE_CANVAS_PROMPT,
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

  it("does not expose a canvas action while the feature is disabled", () => {
    mocks.featureEnabled = false;

    render(<CanvasTaskCreateLauncher workspaceId="workspace-1" />);

    expect(screen.queryByRole("button", { name: CREATE_CANVAS_LABEL })).toBeNull();
    expect(mocks.dialogProps).toBeNull();
  });
});
