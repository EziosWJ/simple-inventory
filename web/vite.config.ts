import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  // Worktrees may share node_modules; each checkout needs its own optimizer cache.
  cacheDir: path.resolve(__dirname, "../.task/vite-cache"),
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});

