import { createContext } from "react";

// Office identifiers are local task references only in Office task discussions.
// Orchestrator conversations can discuss arbitrary external issue systems.
export const ImplicitTaskLinksContext = createContext(true);
