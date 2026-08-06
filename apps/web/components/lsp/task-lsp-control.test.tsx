import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";
import { TaskLspControl, TaskLspDisclosure } from "./task-lsp-control";

const TEST_TIME = "2026-08-05T12:00:00Z";
const KOTLIN_ROW_TEST_ID = "task-lsp-language-kotlin";

const controller = vi.hoisted(() => ({
  languages: [] as TaskLspLanguageSnapshot[],
  pending: {} as Record<string, string>,
  loaded: true,
  loading: false,
  error: null as string | null,
  capacity: { active: 1, queued: 0, limit: 4 },
  start: vi.fn(),
  stop: vi.fn(),
  restart: vi.fn(),
  setPolicy: vi.fn(),
  refetch: vi.fn(),
}));

vi.mock("@/hooks/domains/lsp/use-task-lsp", () => ({
  useTaskLsp: () => controller,
}));

function language(
  id: string,
  overrides: Partial<TaskLspLanguageSnapshot> = {},
): TaskLspLanguageSnapshot {
  return {
    task_id: "task-1",
    language: id,
    policy: "inherit",
    detected: true,
    detection_state: "complete",
    detection_truncated: false,
    phase: "off",
    generation: 0,
    revision: 1,
    last_transition_at: TEST_TIME,
    last_action: "",
    last_initiator: "automatic",
    restart_required: false,
    created_at: TEST_TIME,
    updated_at: TEST_TIME,
    effective_policy: "inherit",
    activity: "idle",
    progress: [],
    ...overrides,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-08-05T12:05:00Z"));
  controller.languages = [
    language("go"),
    language("kotlin", {
      policy: "keep_warm",
      effective_policy: "keep_warm",
      phase: "initializing",
      generation: 3,
      process_started_at: TEST_TIME,
      initialize_started_at: "2026-08-05T12:00:15Z",
      last_action: "restart",
      last_action_at: TEST_TIME,
      last_restart_reason: "user_restart",
      last_initiator: "user",
      activity: "server_work",
      progress: [
        {
          token: "gradle",
          title: "Importing Kotlin project",
          message: "Resolving Gradle model",
          percentage: 42,
          started_at: "2026-08-05T12:01:00Z",
        },
      ],
    }),
  ];
  controller.pending = {};
  controller.error = null;
  for (const action of [
    controller.start,
    controller.stop,
    controller.restart,
    controller.setPolicy,
  ]) {
    action.mockReset().mockResolvedValue(undefined);
  }
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("TaskLspDisclosure evidence", () => {
  it("shows task policy, honest progress, generation, elapsed time, reason, and initiator", () => {
    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    const kotlin = screen.getByTestId(KOTLIN_ROW_TEST_ID);
    expect(kotlin.getAttribute("data-lsp-state")).toBe("server_work");
    expect(kotlin.getAttribute("data-lsp-policy")).toBe("keep_warm");
    expect(kotlin.getAttribute("data-lsp-generation")).toBe("3");
    expect(kotlin.textContent).toContain("Importing Kotlin project");
    expect(kotlin.textContent).toContain("Resolving Gradle model");
    expect(kotlin.textContent).toContain("42%");
    expect(kotlin.textContent).toContain("4 min 00 sec");
    expect(kotlin.textContent).toContain("Cross-file definitions and references may be incomplete");
    expect(kotlin.textContent).toContain("Generation 3");
    expect(kotlin.textContent).toContain("Started 5 min 00 sec ago");
    expect(kotlin.textContent).toContain("Restarted by user");
  });

  it("shows completed server work as idle evidence instead of active progress", () => {
    controller.languages = [
      language("kotlin", {
        phase: "ready",
        generation: 3,
        effective_policy: "keep_warm",
        last_completed_work: {
          token: "gradle",
          title: "Importing Kotlin project",
          message: "Project model loaded",
          started_at: "2026-08-05T12:01:00Z",
          completed_at: "2026-08-05T12:04:00Z",
        },
      }),
    ];

    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    const kotlin = screen.getByTestId(KOTLIN_ROW_TEST_ID);
    expect(kotlin.getAttribute("data-lsp-state")).toBe("ready");
    expect(within(kotlin).queryByTestId("task-lsp-progress")).toBeNull();
    expect(within(kotlin).getByTestId("task-lsp-completed-work").textContent).toContain(
      "Project model loaded",
    );
  });

  it("shows honest long-running initialize evidence without an ETA", () => {
    controller.languages = [
      language("kotlin", {
        phase: "initializing",
        generation: 3,
        effective_policy: "keep_warm",
        process_started_at: TEST_TIME,
        initialize_started_at: "2026-08-05T12:00:15Z",
      }),
    ];

    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    const evidence = screen.getByTestId("task-lsp-initialization");
    expect(evidence.getAttribute("data-lsp-initialization-stage")).toBe("long_running");
    expect(evidence.textContent).toContain("Initialization is taking longer than usual");
    expect(evidence.textContent).toContain("Kotlin LSP may be importing the Gradle project");
    expect(evidence.textContent).not.toMatch(/ETA|time remaining/i);
  });

  it("shows ready/idle evidence when the server reports no active work", () => {
    controller.languages = [
      language("kotlin", {
        phase: "ready",
        generation: 3,
        effective_policy: "keep_warm",
      }),
    ];

    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    expect(screen.getByTestId("task-lsp-idle").textContent).toContain(
      "No background work reported",
    );
  });
});

describe("TaskLspDisclosure controls", () => {
  it("keeps queued, unsupported, and missing-binary states actionable", () => {
    controller.languages = [
      language("go", { phase: "queued", generation: 1, effective_policy: "keep_warm" }),
      language("python", { phase: "unsupported", error_code: "unsupported_executor" }),
      language("kotlin", {
        phase: "error",
        generation: 1,
        error_code: "task_host_control_failed",
        error_message: "kotlin-lsp not found",
      }),
    ];

    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    expect(screen.getByTestId("task-lsp-language-go").textContent).toContain(
      "Waiting for capacity",
    );
    expect(screen.getByTestId("task-lsp-language-python").textContent).toContain(
      "Language servers are not supported by this task executor",
    );
    expect(screen.getByTestId(KOTLIN_ROW_TEST_ID).textContent).toContain(
      "Install kotlin-lsp on the task host",
    );
  });

  it("delegates policy, Start, and Stop to the shared task controller", () => {
    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    fireEvent.change(screen.getByRole("combobox", { name: "Go task policy" }), {
      target: { value: "disabled" },
    });
    fireEvent.click(
      within(screen.getByTestId("task-lsp-language-go")).getByRole("button", { name: "Start" }),
    );
    fireEvent.click(
      within(screen.getByTestId(KOTLIN_ROW_TEST_ID)).getByRole("button", {
        name: "Stop",
      }),
    );

    expect(controller.setPolicy).toHaveBeenCalledWith("go", "disabled");
    expect(controller.start).toHaveBeenCalledWith("go");
    expect(controller.stop).toHaveBeenCalledWith("kotlin");
  });

  it("explains restart impact and waits for confirmation", () => {
    render(<TaskLspDisclosure taskId="task-1" touch={false} />);

    fireEvent.click(
      within(screen.getByTestId(KOTLIN_ROW_TEST_ID)).getByRole("button", {
        name: "Restart",
      }),
    );
    expect(controller.restart).not.toHaveBeenCalled();

    const dialog = screen.getByRole("alertdialog", { name: "Restart Kotlin language server" });
    expect(dialog.textContent).toContain("stops the current Kotlin process");
    expect(dialog.textContent).toContain("project analysis will run again");
    fireEvent.click(within(dialog).getByRole("button", { name: "Restart" }));

    expect(controller.restart).toHaveBeenCalledWith("kotlin");
  });

  it("uses 44px controls in the embedded touch composition without nesting a drawer", () => {
    render(<TaskLspDisclosure taskId="task-1" touch />);

    const disclosure = screen.getByTestId("task-lsp-disclosure");
    expect(disclosure.querySelector("[role=dialog]")).toBeNull();
    expect(disclosure.className).toContain("shrink-0");
    expect(
      within(screen.getByTestId("task-lsp-language-go")).getByRole("button", { name: "Start" })
        .className,
    ).toContain("h-11");
  });

  it("opens one shared disclosure from the compact task control", () => {
    render(
      <TooltipProvider>
        <TaskLspControl taskId="task-1" placement="status-bar" />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("app-status-lsp"));

    expect(screen.getByTestId("task-lsp-surface")).toBeTruthy();
    expect(screen.getAllByTestId("task-lsp-disclosure")).toHaveLength(1);
  });

  it("identifies the current language on the editor shortcut", () => {
    render(
      <TooltipProvider>
        <TaskLspControl taskId="task-1" placement="editor-toolbar" language="kotlin" />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("lsp-status-button").getAttribute("data-lsp-language")).toBe(
      "kotlin",
    );
    expect(screen.getByTestId("lsp-status-button").getAttribute("data-lsp-state")).toBe(
      "server_work",
    );
  });
});
