type LabelPickerProps = {
  options: string[];
  selected: string[];
  onChange: (labels: string[]) => void;
};

/** Lets the user toggle labels on and off from a list of options. */
export function LabelPicker({ options, selected, onChange }: LabelPickerProps) {
  const toggle = (label: string) =>
    onChange(
      selected.includes(label) ? selected.filter((item) => item !== label) : [...selected, label],
    );

  return (
    <fieldset className="label-picker">
      <legend>Labels</legend>
      <div className="label-picker-options">
        {options.map((label) => (
          <button
            type="button"
            key={label}
            className={`label-picker-option ${selected.includes(label) ? "is-selected" : ""}`}
            aria-pressed={selected.includes(label)}
            onClick={() => toggle(label)}
          >
            {label}
          </button>
        ))}
      </div>
    </fieldset>
  );
}
