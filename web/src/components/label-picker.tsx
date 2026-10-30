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
    <fieldset className="mx-0 my-[16px] border-0 p-0 [&_legend]:mb-[8px] [&_legend]:font-medium [&_legend]:text-[#ccd5e1]">
      <legend>Labels</legend>
      <div className="flex max-h-[150px] flex-wrap gap-[6px] overflow-y-auto">
        {options.map((label) => (
          <button
            type="button"
            key={label}
            className={`rounded-full border border-[#303642] bg-[#101216] px-[9px] py-[5px] text-[12px] text-[#a9b6c8] hover:bg-[#20242c] [&.is-selected]:border-[#6685ee] [&.is-selected]:bg-[#202b45] [&.is-selected]:text-[#eaf0ff] ${selected.includes(label) ? "is-selected" : ""}`}
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
