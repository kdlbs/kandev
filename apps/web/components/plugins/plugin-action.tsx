"use client";

import * as React from "react";
import type { PluginActionGroupProps, PluginActionProps } from "@kandev/plugin-sdk";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { SurfaceAction } from "@/components/actions/surface-action";
import { surfaceActionGroupClassName } from "@/components/actions/surface-action-styles";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePluginActionSurface } from "./plugin-action-surface";

function visibleActionText(
  label: string,
  text: unknown,
  icon: React.ReactNode,
): string | undefined {
  if (typeof text === "string" && text.length > 0) return text;
  return React.Children.toArray(icon).length === 0 ? label : undefined;
}

function actionTooltip(tooltip: string | undefined, label: string, iconOnly: boolean) {
  if (tooltip !== undefined) return tooltip || undefined;
  return iconOnly ? label : undefined;
}

function tooltipTrigger(action: React.ReactElement, disabled?: boolean): React.ReactElement {
  if (!disabled) return action;
  return (
    <span
      aria-disabled="true"
      className="inline-flex rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      tabIndex={0}
    >
      {action}
    </span>
  );
}

export function PluginAction(props: PluginActionProps) {
  const surface = usePluginActionSurface();
  const { isFinePointer } = useResponsiveBreakpoint();
  if (!surface) {
    // i18n-exempt: developer misuse error, not user-facing copy.
    throw new Error("host.ui.Action must be rendered inside a supported plugin slot.");
  }

  const label = typeof props.label === "string" ? props.label : "";
  const icon = props.icon as React.ReactNode;
  const visibleText = visibleActionText(label, props.text, icon);
  const badge = typeof props.badge === "string" ? props.badge : undefined;
  const iconOnly = !visibleText;
  const tooltip = actionTooltip(props.tooltip, label, iconOnly);
  const showTooltip = isFinePointer && surface.presentation !== "mobile";

  const action = (
    <SurfaceAction
      surface={surface.surface}
      presentation={surface.presentation}
      label={label}
      icon={icon}
      text={visibleText}
      badge={badge}
      tone={props.tone}
      pressed={props.pressed}
      disabled={props.disabled}
      busy={props.busy}
      ref={props.ref as React.Ref<HTMLButtonElement>}
      id={props.id}
      aria-expanded={props["aria-expanded"]}
      aria-controls={props["aria-controls"]}
      aria-haspopup={props["aria-haspopup"]}
      aria-describedby={props["aria-describedby"]}
      data-testid={props["data-testid"]}
      data-state={props["data-state"]}
      data-side={props["data-side"]}
      data-align={props["data-align"]}
      data-disabled={props["data-disabled"]}
      onClick={props.onClick as React.MouseEventHandler<HTMLButtonElement> | undefined}
      onFocus={props.onFocus as React.FocusEventHandler<HTMLButtonElement> | undefined}
      onBlur={props.onBlur as React.FocusEventHandler<HTMLButtonElement> | undefined}
      onKeyDown={props.onKeyDown as React.KeyboardEventHandler<HTMLButtonElement> | undefined}
      onPointerDown={
        props.onPointerDown as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onPointerUp={props.onPointerUp as React.PointerEventHandler<HTMLButtonElement> | undefined}
      onPointerCancel={
        props.onPointerCancel as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onPointerEnter={
        props.onPointerEnter as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onPointerMove={
        props.onPointerMove as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onPointerLeave={
        props.onPointerLeave as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onLostPointerCapture={
        props.onLostPointerCapture as React.PointerEventHandler<HTMLButtonElement> | undefined
      }
      onMouseEnter={props.onMouseEnter as React.MouseEventHandler<HTMLButtonElement> | undefined}
      onMouseLeave={props.onMouseLeave as React.MouseEventHandler<HTMLButtonElement> | undefined}
    />
  );

  return tooltip && showTooltip ? (
    <Tooltip>
      <TooltipTrigger asChild>{tooltipTrigger(action, props.disabled)}</TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  ) : (
    action
  );
}

export function PluginActionGroup(props: PluginActionGroupProps) {
  const surface = usePluginActionSurface();
  const children = React.Children.toArray(props.children as React.ReactNode);
  if (children.length === 0) return null;

  if (!surface) {
    // i18n-exempt: developer misuse error, not user-facing copy.
    throw new Error("host.ui.ActionGroup must be rendered inside a supported plugin slot.");
  }

  return (
    <div
      className={surfaceActionGroupClassName(surface.surface, surface.presentation)}
      role={props.label ? "group" : undefined}
      aria-label={props.label}
      data-slot="surface-action-group"
      data-surface={surface.surface}
    >
      {children}
    </div>
  );
}
