/* eslint-disable i18next/no-literal-string -- immutable attributed fixtures preserve legacy plugin copy. */
import { useState, type ComponentType, type MouseEventHandler, type ReactNode } from "react";
import type { PluginHostApi } from "@/lib/plugins/types";

type LegacyButtonProps = {
  type?: "button";
  variant?: string;
  size?: string;
  className?: string;
  "aria-label"?: string;
  "aria-expanded"?: boolean;
  "data-testid"?: string;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  children?: ReactNode;
};

type LegacyFixtureProps = {
  host: PluginHostApi;
  slotProps?: unknown;
  onHostButtonActivate?: () => void;
};

function legacyButton(host: PluginHostApi) {
  return host.ui.Button as ComponentType<LegacyButtonProps>;
}

/** Frozen template-shaped host Button fixture. Keep it off the Action API. */
export function LegacyHostButton({
  host,
  onActivate,
}: LegacyFixtureProps & { onActivate?: () => void }) {
  const Button = legacyButton(host);
  return (
    <Button
      type="button"
      variant="outline"
      size="icon-sm"
      className="h-7 w-7"
      aria-label="Legacy host button"
      data-testid="legacy-host-button"
      onClick={onActivate}
    >
      H
    </Button>
  );
}

/** Frozen raw-button fixture shaped after the Task Manager metric trigger. */
export function LegacyRawMetricButton() {
  const [activated, setActivated] = useState(false);
  return (
    <button
      type="button"
      className="h-7 min-w-12 rounded border border-amber-600 bg-amber-500/10 px-2 text-xs"
      aria-label="Legacy CPU usage"
      data-testid="legacy-raw-metric"
      data-activated={activated}
      onClick={() => setActivated((previous) => !previous)}
    >
      CPU 17%
    </button>
  );
}

/** Frozen status-content fixture shaped after the Provider Usage status strip. */
export function LegacyStatusContribution({ slotProps }: { slotProps?: unknown }) {
  const props = slotProps as
    | { placement?: string; presentation?: string; activeTaskId?: string | null }
    | undefined;
  return (
    <span data-testid="legacy-status-contribution">
      {props?.placement ?? "unknown"}:{props?.presentation ?? "unknown"}:
      {props?.activeTaskId ?? "workspace"}
    </span>
  );
}

/** Frozen custom disclosure trigger shaped after the Kandy preview control. */
export function LegacyControlledDisclosure() {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button
        type="button"
        className="h-7 w-7 rounded border border-violet-500 bg-violet-500/10"
        aria-label="Open legacy preview"
        aria-expanded={open}
        aria-controls="legacy-preview-content"
        data-testid="legacy-preview-trigger"
        onClick={() => setOpen((previous) => !previous)}
      >
        K
      </button>
      <div
        id="legacy-preview-content"
        role="region"
        aria-label="Legacy preview"
        data-testid="legacy-preview-content"
        hidden={!open}
      >
        Preview details
      </div>
    </div>
  );
}

export function LegacyActionContribution(props: LegacyFixtureProps) {
  return (
    <>
      <LegacyHostButton host={props.host} onActivate={props.onHostButtonActivate} />
      <LegacyRawMetricButton />
      <LegacyStatusContribution slotProps={props.slotProps} />
      <LegacyControlledDisclosure />
    </>
  );
}
