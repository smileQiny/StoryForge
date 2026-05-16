import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { resolve } from "node:path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("@xterm") || id.includes("/xterm")) {
            return "terminal-vendor";
          }
          if (id.includes("lucide-react")) {
            return "icons-vendor";
          }
          if (id.includes("/node_modules/react/") || id.includes("/node_modules/react-dom/")) {
            return "react-vendor";
          }
          if (id.includes("@base-ui/react")) {
            return "ui-vendor";
          }
          if (id.includes("/node_modules/")) {
            return "vendor";
          }
          return undefined;
        },
      },
    },
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
      "lucide-react": resolve(__dirname, "node_modules/lucide-react/dist/cjs/lucide-react.js"),
    },
  },
  server: {
    port: 4567,
    proxy: {
      "/api": {
        target: `http://localhost:8080`,
        changeOrigin: true,
        ws: true,
      },
    },
  },
});
