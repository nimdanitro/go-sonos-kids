import { writeFileSync } from "node:fs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// `yarn dev` talks to a locally running `sonoskids serve` (default address).
const backend = "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    {
      // emptyOutDir wipes build/, but ui/ui.go embeds it and a fresh checkout
      // needs at least one file there for `go build` to work.
      name: "keep-build-dir",
      closeBundle: () => writeFileSync("build/.gitkeep", ""),
    },
  ],
  // Production output is embedded into the Go binary by ui/ui.go.
  build: { outDir: "build", emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      "/api": backend,
      "/healthz": backend,
    },
  },
  preview: { port: 4173, proxy: { "/api": backend } },
});
