import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
import { z } from "astro/zod";

// Docs live as MDX files in src/docs. `order` sorts the sidebar within a section.
const docs = defineCollection({
  loader: glob({ pattern: "**/*.mdx", base: "./src/docs" }),
  schema: z.object({
    title: z.string(),
    description: z.string(),
    section: z.string(),
    order: z.number(),
  }),
});

export const collections = { docs };
