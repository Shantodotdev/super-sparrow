/** Shows a label's persisted color without relying on its name. */
export function LabelDot({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block size-2 shrink-0 rounded-full"
      style={{ backgroundColor: color }}
    />
  );
}
