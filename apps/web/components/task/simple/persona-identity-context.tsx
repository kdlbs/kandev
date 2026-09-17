import { createContext } from "react";
export const PersonaIdentityContext = createContext<{
  id: string;
  name: string;
  icon?: string;
} | null>(null);

// Feature adapters provide identity lookup without coupling shared chat to their stores.
export const ChatIdentityContext = createContext<Record<string, string>>({});
