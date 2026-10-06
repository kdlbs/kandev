"use client";

import { createContext, useContext } from "react";

const MessageTaskOriginContext = createContext<string | undefined>(undefined);

export const MessageTaskOriginProvider = MessageTaskOriginContext.Provider;
export const useMessageTaskOrigin = () => useContext(MessageTaskOriginContext);
