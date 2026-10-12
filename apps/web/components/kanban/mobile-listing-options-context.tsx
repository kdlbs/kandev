import { createContext, useContext } from "react";

export const MobileListingOptionsContext = createContext<{ close: () => void } | null>(null);
export const useMobileListingOptions = () => useContext(MobileListingOptionsContext);
