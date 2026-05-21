import { reactRouter } from "@react-router/dev/vite";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig(({ mode }) => ({
  plugins: mode === "test" ? [tailwindcss()] : [tailwindcss(), reactRouter()],
  resolve: {
    alias: { "~": new URL("./app", import.meta.url).pathname },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./app/test/setup.ts"],
    globals: true,
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: ["app/**/*.{ts,tsx}"],
      exclude: ["app/test/**", "app/routes.ts", "app/root.tsx"],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
}));
