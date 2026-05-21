import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./app/test/setup.ts"],
    coverage: {
      provider: "v8",
      exclude: ["app/root.tsx", "app/routes.ts"],
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80,
      },
    },
  },
  resolve: {
    alias: {
      "~": path.resolve(__dirname, "./app"),
    },
  },
});
