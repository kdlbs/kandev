"use client";

import { createContext, type ReactNode, useContext } from "react";

export type PluginActionSurfaceName =
  | "topbar"
  | "composer"
  | "sidebar"
  | "status-bar"
  | "status-drawer";

export type PluginActionPresentation = "desktop" | "mobile";

export type PluginActionSurfaceValue = {
  surface: PluginActionSurfaceName;
  presentation?: PluginActionPresentation;
};

export const PluginActionSurfaceContext = createContext<PluginActionSurfaceValue | null>(null);

export function PluginActionSurfaceProvider(props: {
  value: PluginActionSurfaceValue | null;
  children: ReactNode;
}) {
  return (
    <PluginActionSurfaceContext.Provider value={props.value}>
      {props.children}
    </PluginActionSurfaceContext.Provider>
  );
}

export function usePluginActionSurface(): PluginActionSurfaceValue | null {
  return useContext(PluginActionSurfaceContext);
}
