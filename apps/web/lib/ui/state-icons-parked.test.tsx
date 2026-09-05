import { describe, expect, it } from "vitest";
import { isValidElement, type ReactNode } from "react";
import { render } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { CompositorSpin } from "@kandev/ui/compositor-spin";
import {
  IconCheck,
  IconCircleCheck,
  IconCircleFilled,
  IconLoader,
  IconLoader2,
  IconMessageQuestion,
  IconShieldQuestion,
} from "@tabler/icons-react";
import { getSessionStateIcon, getTaskStateIcon } from "./state-icons";

function iconType(node: ReactNode) {
  if (!isValidElement(node)) throw new Error("Expected React element");
  if (node.type === CompositorSpin) {
    return iconType((node.props as { children: ReactNode }).children);
  }
  return node.type;
}

const SPIN_CLASS = "animate-spin";
const SPIN_SELECTOR = `.${SPIN_CLASS}`;

describe("getTaskStateIcon — parked on background work", () => {
  // AC-23/AC-59: parkedOnBackgroundWork renders the same tooltip-carrying
  // affordance as a live foregroundActivity=background aggregate, and
  // defaults to false so every pre-existing call site is unaffected.
  it("renders the accessible background-running icon with the tooltip label", () => {
    const { container } = render(
      <TooltipProvider>
        {getTaskStateIcon("REVIEW", undefined, { parkedOnBackgroundWork: true })}
      </TooltipProvider>,
    );
    const icon = container.querySelector('[data-testid="task-state-background-running"]');
    expect(icon).not.toBeNull();
    expect(container.querySelector('[aria-label="Background work is running"]')).not.toBeNull();
  });

  it("defaults to false: omitting the option does not render the parked affordance", () => {
    expect(iconType(getTaskStateIcon("COMPLETED", undefined))).toBe(IconCheck);
    expect(iconType(getTaskStateIcon("COMPLETED", undefined, {}))).toBe(IconCheck);
  });

  // AC-34: pending-input and any live foregroundActivity outrank parked.
  it("keeps pending clarification and permission over the parked marker", () => {
    expect(
      iconType(
        getTaskStateIcon("REVIEW", undefined, {
          hasPendingClarification: true,
          parkedOnBackgroundWork: true,
        }),
      ),
    ).toBe(IconMessageQuestion);
    expect(
      iconType(
        getTaskStateIcon("REVIEW", undefined, {
          hasPendingPermission: true,
          parkedOnBackgroundWork: true,
        }),
      ),
    ).toBe(IconShieldQuestion);
  });

  it("keeps a live generating or background aggregate over the parked marker", () => {
    expect(
      iconType(
        getTaskStateIcon("REVIEW", undefined, {
          foregroundActivity: "generating",
          parkedOnBackgroundWork: true,
        }),
      ),
    ).toBe(IconLoader2);
    const bg = getTaskStateIcon("REVIEW", undefined, {
      foregroundActivity: "background",
      parkedOnBackgroundWork: true,
    });
    expect(iconType(bg)).toBe(IconLoader);
  });

  it("outranks a stale WAITING_FOR_INPUT coarse task state (no explicit pending flag)", () => {
    const { container } = render(
      <TooltipProvider>
        {getTaskStateIcon("WAITING_FOR_INPUT", undefined, { parkedOnBackgroundWork: true })}
      </TooltipProvider>,
    );
    expect(container.querySelector('[data-testid="task-state-background-running"]')).not.toBeNull();
  });
});

describe("getSessionStateIcon — parked on background work (AC-51/52)", () => {
  it("shows the background-work spinner for a settled session with a positive liveness sample", () => {
    const icon = getSessionStateIcon("WAITING_FOR_INPUT", undefined, {
      parkedOnBackgroundWork: true,
    });
    expect(iconType(icon)).toBe(IconLoader2);
    const { container } = render(<>{icon}</>);
    expect(container.querySelector(SPIN_SELECTOR)).not.toBeNull();
  });

  it("reads identically to a live foregroundActivity=background session", () => {
    const live = iconType(
      getSessionStateIcon("RUNNING", undefined, { foregroundActivity: "background" }),
    );
    const parked = iconType(
      getSessionStateIcon("RUNNING", undefined, { parkedOnBackgroundWork: true }),
    );
    expect(parked).toBe(live);
  });

  it("defaults to false so existing call sites are unaffected", () => {
    expect(iconType(getSessionStateIcon("WAITING_FOR_INPUT"))).toBe(IconMessageQuestion);
    expect(iconType(getSessionStateIcon("RUNNING"))).toBe(IconCircleFilled);
  });

  it("lets pending clarification win over parked-on-background-work", () => {
    expect(
      iconType(
        getSessionStateIcon("RUNNING", undefined, {
          hasPendingClarification: true,
          parkedOnBackgroundWork: true,
        }),
      ),
    ).toBe(IconMessageQuestion);
  });

  it("lets pending permission win over parked-on-background-work", () => {
    expect(
      iconType(
        getSessionStateIcon("WAITING_FOR_INPUT", undefined, {
          hasPendingPermission: true,
          parkedOnBackgroundWork: true,
        }),
      ),
    ).toBe(IconShieldQuestion);
  });

  it("does not let a stale parked reading mask a terminal session state", () => {
    // Mirrors the foregroundActivity=background terminal-state test: canRequestInput
    // is false once the session is COMPLETED, so parked can never resurrect a spinner.
    expect(
      iconType(getSessionStateIcon("COMPLETED", undefined, { parkedOnBackgroundWork: true })),
    ).toBe(IconCircleCheck);
  });
});
