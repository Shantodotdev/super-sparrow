/** Turns a display value such as "In Review" into a CSS class suffix such as "in-review". */
export function slugify(value: string): string {
  return value.toLowerCase().replace(/\s+/g, "-");
}
