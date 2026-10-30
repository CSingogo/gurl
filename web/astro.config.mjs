// @ts-check
import { defineConfig, fontProviders } from "astro/config";
import mdx from "@astrojs/mdx";
import react from "@astrojs/react";
import sitemap from "@astrojs/sitemap";
import tailwindcss from "@tailwindcss/vite";

// https://astro.build/config
export default defineConfig({
  site: "https://gurl.somosingogo.dev",
  output: "static",
  redirects: { "/docs": "/docs/getting-started" },
  integrations: [react(), mdx(), sitemap()],
  vite: { plugins: [tailwindcss()] },
  markdown: { shikiConfig: { theme: "github-dark-default" } },
  fonts: [
    { provider: fontProviders.google(), name: "Geist", cssVariable: "--font-geist-sans" },
    { provider: fontProviders.google(), name: "Geist Mono", cssVariable: "--font-geist-mono" },
    // Wordmark face (the logo only). Appwrite's Aeonik Pro is paid; Hanken Grotesk is the closest free match.
    { provider: fontProviders.google(), name: "Hanken Grotesk", cssVariable: "--font-wordmark", weights: [600], styles: ["normal"] },
  ],
});
