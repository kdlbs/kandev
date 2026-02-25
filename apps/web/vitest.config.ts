import path from "path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
      "@kandev/ui": path.resolve(__dirname, "../packages/ui/src"),
      "@kandev/theme": path.resolve(__dirname, "../packages/theme/src"),
    },
  },
  test: {
    environment: "happy-dom",
  },
});
