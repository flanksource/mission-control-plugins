import path from "path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "./",
  plugins: [react()],
  define: {
    __PLUGIN_VERSION__: JSON.stringify(process.env.PLUGIN_VERSION ?? "dev"),
    __PLUGIN_BUILD_DATE__: JSON.stringify(process.env.PLUGIN_BUILD_DATE ?? ""),
  },
  build: {
    outDir: path.resolve(__dirname, "../ui"),
    emptyOutDir: true,
    minify: process.env.PLUGIN_UI_RELEASE === "1",
    sourcemap: true,
    rollupOptions: {
      input: path.resolve(__dirname, "index.html"),
      output: {
        entryFileNames: "assets/[name].js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
});
