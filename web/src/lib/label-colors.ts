/** Label colors drawn from the application's existing badge and status palette. */
export const labelColors = [
  { name: "Blue", value: "#6c9bff" },
  { name: "Purple", value: "#a77bff" },
  { name: "Green", value: "#3fd9a0" },
  { name: "Yellow", value: "#ffc933" },
  { name: "Orange", value: "#ff9442" },
  { name: "Red", value: "#ff6b6b" },
  { name: "Cyan", value: "#2ed3e6" },
  { name: "Gray", value: "#9aa8bd" },
] as const;

/** A soft diagonal gradient from a lighter tint of the color to the color itself. */
export function labelGradient(color: string): string {
  return `linear-gradient(135deg, color-mix(in srgb, ${color} 55%, white), ${color} 70%)`;
}
