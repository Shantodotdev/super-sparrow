/** The destinations offered by the command palette. */
export const paletteLinks = [
  { to: "/issues", label: "My issues" },
  { to: "/activity", label: "Activity" },
  { to: "/$projectSlug/issues", label: "Issues" },
  { to: "/$projectSlug/milestones", label: "Milestones" },
  { to: "/settings", label: "People and agents" },
] as const;
