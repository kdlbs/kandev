const DEBUG = false;

export const log = (...args: unknown[]) => {
  if (DEBUG) console.log("[PassthroughTerminal]", ...args);
};
