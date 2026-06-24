import { defineConfig, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

const backendTarget = "http://127.0.0.1:9192";
const backendProxy: Record<string, ProxyOptions> = {
  "/api": { target: backendTarget, xfwd: true },
  "/p": { target: backendTarget, xfwd: true },
  "/admin/api": { target: backendTarget, xfwd: true },
};

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 9191,
    proxy: backendProxy,
  },
  preview: {
    host: "0.0.0.0",
    port: 9191,
    proxy: backendProxy,
  },
});
