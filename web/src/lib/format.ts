/** Turns a value such as "in_review" or "Should fix" into a CSS class suffix such as "in-review". */
export function slugify(value: string): string {
  return value.toLowerCase().replace(/[\s_]+/g, "-");
}

const dateFormat = new Intl.DateTimeFormat("en", {
  month: "short",
  day: "numeric",
  year: "numeric",
});
const timeFormat = new Intl.DateTimeFormat("en", { hour: "numeric", minute: "2-digit" });

/** Formats an API timestamp as a date, such as "Nov 6, 2024". */
export function formatDate(timestamp: string): string {
  return dateFormat.format(new Date(timestamp));
}

/** Formats an API timestamp as a time of day, such as "11:28 AM". */
export function formatTime(timestamp: string): string {
  return timeFormat.format(new Date(timestamp));
}

/** Shortens a commit SHA to the seven characters git shows. */
export function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

/** Returns url when it is an http or https URL, which is safe to link to, and "" otherwise. */
export function webUrl(url: string): string {
  return /^https?:\/\//i.test(url) ? url : "";
}
