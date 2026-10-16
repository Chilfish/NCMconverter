import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "NCMconverter",
  identifier: "dev.chilfish.ncmconverter",
  version: "0.1.0",
  // The Go package of the app, relative to the project root.
  main: "./cmd/ncmconverter",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  // The frontend lives in desktop/, and Vite's root points there, so the
  // built page lands in desktop/dist.
  frontendDist: "desktop/dist",
  bindings: "desktop/src/mygo.ts",
  // Matches the dist/ entry in .gitignore, and keeps the CLI's own bin/ apart.
  out: "dist",
  // Both default to ad hoc (macOS) and unsigned (Windows), so a build without
  // secrets still works. Set these in CI to sign and notarize a real release.
  macos: { signingIdentity: process.env.MACOS_SIGNING_IDENTITY },
  windows: { certificate: process.env.WINDOWS_CERTIFICATE_FILE },
});
