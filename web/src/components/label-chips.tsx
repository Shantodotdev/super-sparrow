export function LabelChips({ labels }: { labels: string[] }) {
  return (
    <span className="issue-labels">
      {labels.map((label) => (
        <span className={`label-chip label-${label.toLowerCase()}`} key={label}>
          {label}
        </span>
      ))}
    </span>
  );
}
