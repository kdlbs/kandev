import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { emitSettingsTargetRequest } from "@/lib/settings-discovery/target";
import { SettingsCard } from "./settings-card";
import { SettingsTargetProvider } from "./settings-target-provider";
import {
  WorkflowCardDisclosureBody,
  useExpandOnSettingsTarget,
  useWorkflowCardDisclosure,
} from "./workflow-card-disclosure";

const TARGET = "workflow-card-wf-1";

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});

describe("useWorkflowCardDisclosure", () => {
  it("starts collapsed in the compact view", () => {
    const { result } = renderHook(() =>
      useWorkflowCardDisclosure({ compact: true, initiallyExpanded: false }),
    );
    expect(result.current.open).toBe(false);
    expect(result.current.mounted).toBe(false);
  });

  it("starts expanded for draft workflows", () => {
    const { result } = renderHook(() =>
      useWorkflowCardDisclosure({ compact: true, initiallyExpanded: true }),
    );
    expect(result.current.open).toBe(true);
    expect(result.current.mounted).toBe(true);
  });

  it("is always open in the detailed view", () => {
    const { result } = renderHook(() =>
      useWorkflowCardDisclosure({ compact: false, initiallyExpanded: false }),
    );
    expect(result.current.open).toBe(true);
    expect(result.current.mounted).toBe(true);
  });

  it("toggles and keeps the body mounted after the first open", () => {
    const { result } = renderHook(() =>
      useWorkflowCardDisclosure({ compact: true, initiallyExpanded: false }),
    );
    act(() => result.current.toggle());
    expect(result.current.open).toBe(true);
    act(() => result.current.toggle());
    expect(result.current.open).toBe(false);
    expect(result.current.mounted).toBe(true);
  });
});

function DraftInput() {
  const [value, setValue] = useState("");
  return <input aria-label="draft" value={value} onChange={(e) => setValue(e.target.value)} />;
}

function Harness() {
  const disclosure = useWorkflowCardDisclosure({ compact: true, initiallyExpanded: false });
  return (
    <>
      <button type="button" onClick={disclosure.toggle}>
        toggle
      </button>
      <WorkflowCardDisclosureBody id="body" open={disclosure.open} mounted={disclosure.mounted}>
        <DraftInput />
      </WorkflowCardDisclosureBody>
    </>
  );
}

describe("WorkflowCardDisclosureBody", () => {
  it("mounts lazily and preserves body state across collapse", () => {
    render(<Harness />);
    expect(screen.queryByLabelText("draft")).toBeNull();

    fireEvent.click(screen.getByText("toggle"));
    fireEvent.change(screen.getByLabelText("draft"), { target: { value: "Renamed" } });

    fireEvent.click(screen.getByText("toggle"));
    const body = document.getElementById("body");
    expect(body?.hidden).toBe(true);

    fireEvent.click(screen.getByText("toggle"));
    expect(body?.hidden).toBe(false);
    expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("Renamed");
  });
});

describe("useExpandOnSettingsTarget", () => {
  it("expands when the page loads with the card's fragment", () => {
    window.history.replaceState(null, "", `/settings#${TARGET}`);
    const expand = vi.fn();
    renderHook(() => useExpandOnSettingsTarget(TARGET, expand));
    expect(expand).toHaveBeenCalledTimes(1);
  });

  it("expands on a matching settings target request only", () => {
    const expand = vi.fn();
    renderHook(() => useExpandOnSettingsTarget(TARGET, expand));
    act(() => emitSettingsTargetRequest("workflow-card-other"));
    expect(expand).not.toHaveBeenCalled();
    act(() => emitSettingsTargetRequest(TARGET));
    expect(expand).toHaveBeenCalledTimes(1);
  });

  it("expands when the fragment changes to the card", () => {
    const expand = vi.fn();
    renderHook(() => useExpandOnSettingsTarget(TARGET, expand));
    act(() => {
      window.history.replaceState(null, "", `/settings#${TARGET}`);
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(expand).toHaveBeenCalledTimes(1);
  });
});

function TargetedWorkflowCardContent() {
  const disclosure = useWorkflowCardDisclosure({
    compact: true,
    initiallyExpanded: false,
    targetId: TARGET,
  });
  return (
    <SettingsCard
      discoveryTargetId={disclosure.discoveryTargetId}
      data-expanded={disclosure.open}
    />
  );
}

function TargetedWorkflowSettings({
  reveal,
  showCard,
}: {
  reveal: (element: HTMLElement) => void;
  showCard: boolean;
}) {
  return (
    <SettingsTargetProvider revealTarget={reveal}>
      {showCard ? <TargetedWorkflowCardContent /> : null}
    </SettingsTargetProvider>
  );
}

describe("workflow discovery target reveal", () => {
  it("reveals the workflow after it expands for a pending route target", () => {
    window.history.replaceState({}, "", `/settings#${TARGET}`);
    const expandedStatesAtReveal: string[] = [];
    const reveal = vi.fn((element: HTMLElement) => {
      expandedStatesAtReveal.push(element.getAttribute("data-expanded") ?? "");
    });
    const view = render(<TargetedWorkflowSettings reveal={reveal} showCard={false} />);
    act(() => emitSettingsTargetRequest(TARGET));
    view.rerender(<TargetedWorkflowSettings reveal={reveal} showCard />);

    expect(reveal).toHaveBeenCalledTimes(1);
    expect(expandedStatesAtReveal).toEqual(["true"]);
  });
});

describe("useWorkflowCardDisclosure expand", () => {
  it("opens and mounts a collapsed card", () => {
    const { result } = renderHook(() =>
      useWorkflowCardDisclosure({ compact: true, initiallyExpanded: false }),
    );
    act(() => result.current.expand());
    expect(result.current.open).toBe(true);
    expect(result.current.mounted).toBe(true);
  });
});

describe("useWorkflowCardDisclosure across view changes", () => {
  it("keeps a body opened in the detailed view mounted after switching to compact", () => {
    const { result, rerender } = renderHook(
      ({ compact }) => useWorkflowCardDisclosure({ compact, initiallyExpanded: false }),
      { initialProps: { compact: false } },
    );
    expect(result.current.mounted).toBe(true);
    rerender({ compact: true });
    expect(result.current.open).toBe(false);
    expect(result.current.mounted).toBe(true);
  });
});
