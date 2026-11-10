const maxTitleLength = 200;

/** A title and Markdown body read from what a person typed into quick capture. */
export type CaptureText = { title: string; body: string };

/**
 * Splits a quick capture description into a title and a body. The first line with words becomes
 * the title, without its Markdown, and the rest becomes the body. A description of only images
 * is titled "Screenshot" and keeps the images as its body.
 */
export function splitCapture(markdown: string): CaptureText {
  const lines = markdown.split("\n");
  const index = lines.findIndex((line) => plainText(line) !== "");
  if (index < 0) {
    return { title: markdown.trim() ? "Screenshot" : "", body: markdown.trim() };
  }

  const title = shorten(plainText(lines[index]!));
  const rest = [...lines.slice(0, index), ...lines.slice(index + 1)].join("\n").trim();
  return { title, body: rest };
}

/** Reads one Markdown line as plain text: images dropped, links reduced to their words. */
function plainText(line: string): string {
  return line
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/^\s*(?:#{1,6}\s+|>\s*|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)/, "")
    .replace(/[*_~`]/g, "")
    .replace(/\\(.)/g, "$1")
    .trim();
}

/** Cuts a title to the length the server accepts, at a word boundary when one is near. */
function shorten(title: string): string {
  if (title.length <= maxTitleLength) return title;
  const cut = title.slice(0, maxTitleLength - 1);
  const space = cut.lastIndexOf(" ");
  return `${space > maxTitleLength * 0.6 ? cut.slice(0, space) : cut}…`;
}
