import { readFile } from "node:fs/promises";
import path from "node:path";

/**
 * Stylesheet for the zero-JS public receipt: tokens + fonts + receipt + document,
 * in that order, inlined into the HTML so the page renders with a single request
 * (fonts are fetched separately and shared with the app). Files are traced into
 * the standalone build via `outputFileTracingIncludes` in next.config.ts.
 */
const FILES = ["tokens.css", "fonts.css", "receipt.css", "document.css"] as const;

let cached: Promise<string> | null = null;

export function receiptCss(): Promise<string> {
  // Re-read on every request in development so CSS edits show up without a restart.
  if (cached && process.env.NODE_ENV === "production") return cached;
  cached = Promise.all(FILES.map((f) => readFile(path.join(process.cwd(), "src", "styles", f), "utf8"))).then((parts) => minify(parts.join("\n")));
  return cached;
}

/** Comment stripping and whitespace collapse; enough to keep the document small. */
function minify(css: string): string {
  return css
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\s*\n\s*/g, "")
    .replace(/\s{2,}/g, " ")
    .replace(/\s*([{}:;,])\s*/g, "$1")
    .replace(/;}/g, "}")
    .trim();
}
