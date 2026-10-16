import { defineConfig } from "vite";

export default defineConfig({
  // The page lives in desktop/, so Vite's root does too: desktop/index.html is
  // the entry, and builds land in desktop/dist, which is frontendDist in
  // mygo.config.ts.
  root: "desktop",
  server: {
    // The app loads this address during `mygo dev` (devUrl in mygo.config.ts).
    port: 5173,
    strictPort: true,
    // The development app and the packaged builds are not the page's.
    watch: { ignored: ["**/.mygo/**", "**/dist/**"] },
  },
});
