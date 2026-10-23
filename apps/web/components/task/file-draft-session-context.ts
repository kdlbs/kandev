import { createContext, type MutableRefObject } from "react";

export type FileDraftSessionOwner = {
  sessionIdRef: MutableRefObject<string | null>;
  visitRef: MutableRefObject<symbol | null>;
};

export const FileDraftSessionContext = createContext<FileDraftSessionOwner | null>(null);
