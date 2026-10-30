import tailwindcss from "@tailwindcss/vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { defineConfig, loadEnv } from "vite";

const repositoryRoot = resolve(import.meta.dirname, "..");

/** Reads a setting the development server needs, and stops with its name when it is missing. */
function requireSetting(env: Record<string, string>, name: string): string {
  const value = env[name]?.trim();
  if (!value) throw new Error(`${name} is required in the repository .env`);

  return value;
}

export default defineConfig(({ command, mode, isPreview }) => {
  const plugins = [tailwindcss(), tanstackStart({ spa: { enabled: true } }), react()];

  // A production build is static files the Go server serves, so it needs no settings. The build
  // also starts a preview server to render the SPA shell page, which needs none either.
  if (command === "build" || isPreview) return { plugins };

  const env = { ...loadEnv(mode, repositoryRoot, ""), ...process.env } as Record<string, string>;
  const port = Number(requireSetting(env, "WEB_PORT"));
  if (!Number.isInteger(port) || port <= 0) throw new Error("WEB_PORT must be a port number");

  return {
    plugins,
    server: {
      host: "127.0.0.1",
      port,
      strictPort: true,
      // The browser calls the API on the same origin, as it does in production.
      proxy: { "/api": `http://${requireSetting(env, "LISTEN_ADDR")}` },
    },
  };
});
