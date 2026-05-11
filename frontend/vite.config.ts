/// <reference types="vitest" />

import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig(() => {
  const allowedHosts = (process.env.VITE_ALLOWED_HOSTS || "")
    .split(",")
    .map((host) => host.trim())
    .filter(Boolean);

  return {
    plugins: [react()],
    server: {
      allowedHosts,
      proxy: {
        "/api": {
          target: process.env.VITE_API_PROXY_TARGET || "http://127.0.0.1:8090",
          changeOrigin: true,
        },
      },
    },
    build: {
      sourcemap: false,
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts",
      css: true,
    },
  };
});
