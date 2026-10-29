import { slugify } from "../lib/format";
import type { FindingSeverity } from "../lib/types";

export function SeverityBadge({ value }: { value: FindingSeverity }) {
  return <span className={`severity severity-${slugify(value)}`}>{value}</span>;
}
