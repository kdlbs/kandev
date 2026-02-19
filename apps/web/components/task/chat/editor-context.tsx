"use client";

import { createContext, useContext } from "react";

type EditorContextValue = {
  sessionId: string | null;
  taskId: string | null;
};

const EditorContext = createContext<EditorContextValue>({ sessionId: null, taskId: null });

export const EditorContextProvider = EditorContext.Provider;
export const useEditorContext = () => useContext(EditorContext);
