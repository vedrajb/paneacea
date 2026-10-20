export type LinkMatch = { text: string; start: number; end: number };

// http(s) URLs, file:/// URLs, and Windows absolute paths (C:\dir\file or C:/dir/file).
const linkPattern =
  /(?:https?:\/\/|file:\/\/\/?)[^\s"'<>`|^]+|\b[A-Za-z]:[\\/][^\s"'<>`|*?:]*[^\s"'<>`|*?:.,;:)\]}]/g;

/** Finds clickable links in one terminal line. `start` is inclusive, `end` exclusive (0-based). */
export function findLinks(line: string): LinkMatch[] {
  const matches: LinkMatch[] = [];
  for (const m of line.matchAll(linkPattern)) {
    const text = m[0].replace(/[.,;:)\]}]+$/, "");
    if (!text) continue;
    matches.push({ text, start: m.index, end: m.index + text.length });
  }
  return matches;
}
