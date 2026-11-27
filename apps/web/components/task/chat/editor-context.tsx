"use client";

import { createContext, useContext } from "react";

type EditorContextValue = {
  sessionId: string | null;
  taskId: string | null;
  detailActive: boolean;
};

const EditorContext = createContext<EditorContextValue>({
  sessionId: null,
  taskId: null,
  detailActive: true,
});

export const EditorContextProvider = EditorContext.Provider;
export const useEditorContext = () => useContext(EditorContext);
