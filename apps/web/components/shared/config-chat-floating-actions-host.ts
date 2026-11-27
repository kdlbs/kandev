import { createContext, useContext } from "react";

export const ConfigChatFloatingActionsContext = createContext<HTMLElement | null>(null);

export function useConfigChatFloatingActionsHost(): HTMLElement | null {
  return useContext(ConfigChatFloatingActionsContext);
}
