import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import type { WorkflowMovePreviewResponse } from "@/lib/api";
import {
  CompactWorkflowMovePreview,
  WorkflowMovePreviewDisclosure,
  workflowMovePreviewChangeCount,
} from "./workflow-move-preview";

const PREVIEW_DETAILS_TEST_ID = "workflow-move-preview-details";

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
});

function makePreview(
  overrides: Partial<WorkflowMovePreviewResponse> = {},
): WorkflowMovePreviewResponse {
  return {
    task_id: "task-1",
    workflow_step_id: "step-review",
    source_session_id: "session-source",
    evaluated_at: "2026-09-14T00:00:00Z",
    outcome: "reuse_current",
    recipient: {
      session_id: "session-source",
      session_name: "Analysis conversation",
      profile_id: "profile-luna",
      profile_name: "Luna",
      agent_family: "codex",
    },
    model: {
      before: { known: true, label: "gpt-5.6-astra" },
      after: { known: true, label: "gpt-5.6-luna" },
      before_source: "override",
      after_source: "step_rule",
    },
    changes: [
      {
        key: "reasoning_effort",
        label: "reasoning_effort",
        before: "medium",
        after: "max",
        applicability: "planned",
      },
    ],
    context_reset: true,
    context_reset_state: "planned",
    source_disposition: "park",
    dispatch: "prompt",
    ...overrides,
  };
}

function makeState(preview: WorkflowMovePreviewResponse | null) {
  return {
    status: preview ? ("success" as const) : ("loading" as const),
    preview,
    error: null,
    retry: vi.fn(),
  };
}

function makeSuccessState(preview = makePreview()) {
  return {
    status: "success" as const,
    preview,
    error: null,
    retry: vi.fn(),
  };
}

describe("CompactWorkflowMovePreview", () => {
  it("hides the preview while it is idle", () => {
    render(
      <CompactWorkflowMovePreview
        state={{ status: "idle", preview: null, error: null, retry: vi.fn() }}
        isTouchSurface={false}
        expanded={false}
      />,
    );

    expect(screen.queryByTestId("workflow-move-preview")).toBeNull();
    expect(screen.queryByTestId("workflow-move-preview-loading")).toBeNull();
    expect(screen.queryByTestId("workflow-move-preview-error")).toBeNull();
  });

  it("renders localized loading and retryable error states", async () => {
    await i18n.changeLanguage("pt-pt");
    const retry = vi.fn();
    const { rerender } = render(
      <CompactWorkflowMovePreview
        state={{ status: "loading", preview: null, error: null, retry }}
        isTouchSurface
        expanded={false}
      />,
    );

    expect(screen.getByTestId("workflow-move-preview-loading").textContent).toContain(
      "A verificar a sessão...",
    );

    rerender(
      <CompactWorkflowMovePreview
        state={{ status: "error", preview: null, error: new Error("offline"), retry }}
        isTouchSurface
        expanded={false}
      />,
    );

    expect(screen.getByTestId("workflow-move-preview-error").textContent).toContain(
      "Pré-visualização indisponível",
    );
    fireEvent.click(screen.getByTestId("workflow-move-preview-retry"));
    expect(retry).toHaveBeenCalledOnce();
  });

  it("shows the compact success summary without details by default", () => {
    render(
      <CompactWorkflowMovePreview
        state={makeSuccessState()}
        isTouchSurface={false}
        expanded={false}
      />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Reuse current session · gpt-5.6-astra to gpt-5.6-luna · +2 changes",
    );
    expect(screen.queryByTestId(PREVIEW_DETAILS_TEST_ID)).toBeNull();
  });

  it("reveals the full localized details when expanded", async () => {
    await i18n.changeLanguage("pt-pt");
    render(
      <CompactWorkflowMovePreview state={makeSuccessState()} isTouchSurface={false} expanded />,
    );

    expect(screen.getByTestId(PREVIEW_DETAILS_TEST_ID)).toBeTruthy();
    expect(screen.getByText("Analysis conversation")).toBeTruthy();
    expect(screen.getByText("Luna")).toBeTruthy();
    expect(screen.getByText(/Esforço de raciocínio/)).toBeTruthy();
    expect(screen.getByText(/Enviar o prompt do passo/)).toBeTruthy();
  });
});

describe("WorkflowMovePreviewDisclosure", () => {
  it("keeps the compact summary to the recipient and model lines", () => {
    render(
      <WorkflowMovePreviewDisclosure state={makeState(makePreview())} isTouchSurface={false} />,
    );

    expect(screen.getByTestId("workflow-move-preview")).toBeTruthy();
    expect(screen.getByText("Reuse current session")).toBeTruthy();
    expect(screen.getByText("gpt-5.6-astra to gpt-5.6-luna")).toBeTruthy();
    expect(screen.getByText("+2 changes")).toBeTruthy();
    expect(screen.queryByTestId(PREVIEW_DETAILS_TEST_ID)).toBeNull();
  });

  it("shows the full planned change and recipient details on demand", () => {
    render(
      <WorkflowMovePreviewDisclosure state={makeState(makePreview())} isTouchSurface={false} />,
    );

    fireEvent.click(screen.getByTestId("workflow-move-preview-details-toggle"));

    expect(screen.getByTestId(PREVIEW_DETAILS_TEST_ID)).toBeTruthy();
    expect(screen.getByText("Analysis conversation")).toBeTruthy();
    expect(screen.getByText("Luna")).toBeTruthy();
    expect(screen.getByText(/Reasoning effort/)).toBeTruthy();
    expect(screen.getByText(/Medium/)).toBeTruthy();
    expect(screen.getByText(/Maximum/)).toBeTruthy();
    expect(screen.getByText(/Reset/)).toBeTruthy();
    expect(screen.getByText(/Park/)).toBeTruthy();
    expect(screen.getByText(/Send step prompt/)).toBeTruthy();
  });

  it("labels a no-session dispatch separately from a suppressed prompt", () => {
    render(
      <WorkflowMovePreviewDisclosure
        state={makeState(makePreview({ outcome: "no_session", dispatch: "no_session" }))}
        isTouchSurface={false}
      />,
    );

    fireEvent.click(screen.getByTestId("workflow-move-preview-details-toggle"));

    expect(screen.getByText("Agent not started")).toBeTruthy();
  });
});

describe("WorkflowMovePreviewDisclosure model and retry states", () => {
  it("prioritizes unknown models and identifies retained overrides", () => {
    const { rerender } = render(
      <WorkflowMovePreviewDisclosure
        state={makeState(
          makePreview({
            model: {
              before: { known: true, label: "gpt-5.6-luna" },
              after: { known: false },
              after_source: "unknown",
            },
          }),
        )}
        isTouchSurface={false}
      />,
    );
    expect(screen.getByText("Model not known")).toBeTruthy();
    expect(screen.queryByText(/gpt-5.6-luna to/)).toBeNull();

    rerender(
      <WorkflowMovePreviewDisclosure
        state={makeState(
          makePreview({
            model: {
              before: { known: true, label: "gpt-5.6-astra" },
              after: { known: true, label: "gpt-5.6-astra" },
              after_source: "override",
            },
          }),
        )}
        isTouchSurface={false}
      />,
    );
    expect(screen.getByText(/override retained/)).toBeTruthy();
  });

  it("labels a profile model as planned before a destination session exists", () => {
    render(
      <WorkflowMovePreviewDisclosure
        state={makeState(
          makePreview({
            outcome: "create_new",
            model: {
              before: { known: false },
              after: { known: true, label: "gpt-5.6-terra" },
              after_source: "profile",
            },
          }),
        )}
        isTouchSurface={false}
      />,
    );

    expect(screen.getByTestId("workflow-move-preview").textContent).toContain(
      "gpt-5.6-terra (planned)",
    );
  });

  it("keeps move available while loading or retrying an unavailable preview", () => {
    const retry = vi.fn();
    const { rerender } = render(
      <WorkflowMovePreviewDisclosure
        state={{ status: "loading", preview: null, error: null, retry }}
        isTouchSurface
      />,
    );
    expect(screen.getByTestId("workflow-move-preview-loading")).toBeTruthy();

    rerender(
      <WorkflowMovePreviewDisclosure
        state={{ status: "error", preview: null, error: new Error("offline"), retry }}
        isTouchSurface
      />,
    );
    fireEvent.click(screen.getByTestId("workflow-move-preview-retry"));
    expect(retry).toHaveBeenCalledOnce();
    expect(screen.getByTestId("workflow-move-preview-retry").className).toContain("min-h-11");
  });
});

describe("WorkflowMovePreviewDisclosure localization", () => {
  it("localizes backend field, value, and diagnostic codes", async () => {
    await i18n.changeLanguage("pt-pt");
    render(
      <WorkflowMovePreviewDisclosure
        state={makeState(
          makePreview({
            changes: [
              {
                key: "reasoning_effort",
                label: "reasoning_effort",
                before: "medium",
                after: "max",
                applicability: "planned",
              },
            ],
            notices: [
              { code: "missing_original_snapshot" },
              { code: "ambiguous_session_configuration" },
              { code: "session_configuration_skipped" },
            ],
          }),
        )}
        isTouchSurface={false}
      />,
    );

    fireEvent.click(screen.getByTestId("workflow-move-preview-details-toggle"));

    expect(screen.getByText(/Esforço de raciocínio/)).toBeTruthy();
    expect(screen.getByText(/Médio/)).toBeTruthy();
    expect(screen.getByText(/Máximo/)).toBeTruthy();
    expect(screen.getByText("As definições da sessão original estão indisponíveis.")).toBeTruthy();
    expect(
      screen.getByText("Mais do que uma regra de definições da sessão corresponde."),
    ).toBeTruthy();
    expect(screen.getByText("A definição da sessão foi ignorada para esta conversa.")).toBeTruthy();
  });
});

describe("workflowMovePreviewChangeCount", () => {
  it("counts planned and uncertain non-model effects once", () => {
    const preview = makePreview({
      changes: [
        { key: "model", label: "Model", before: "A", after: "B", applicability: "planned" },
        {
          key: "reasoning",
          label: "Reasoning",
          before: "low",
          after: "high",
          applicability: "planned",
        },
        {
          key: "verbosity",
          label: "Verbosity",
          before: "low",
          after: "high",
          applicability: "unknown",
        },
        { key: "same", label: "Same", before: "x", after: "x", applicability: "unchanged" },
        { key: "skipped", label: "Skipped", before: "x", after: "y", applicability: "skipped" },
      ],
      context_reset: true,
      context_reset_state: "planned",
    });
    expect(workflowMovePreviewChangeCount(preview)).toBe(3);
  });
});
