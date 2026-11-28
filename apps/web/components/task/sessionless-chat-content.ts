"use client";

import { createContext, type ReactNode } from "react";

export const SessionlessChatContentContext = createContext<{
  taskId: string;
  content: ReactNode;
} | null>(null);
