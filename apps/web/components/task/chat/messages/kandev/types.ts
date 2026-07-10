// Shared types used by Kandev tool renderers.
//
// `RendererProps` is what the dispatcher passes to every per-tool renderer.
// Renderers receive the parsed args and result (already unwrapped from the MCP
// envelope) plus the tool status, and are responsible for returning a fully
// composed `<KandevRow>` element.

import type { ReactElement } from "react";
import type { KandevStatus } from "./shared";

export type KandevRendererProps = {
  args: Record<string, unknown> | undefined;
  result: unknown;
  status: KandevStatus;
};

export type KandevRenderer = (props: KandevRendererProps) => ReactElement;
