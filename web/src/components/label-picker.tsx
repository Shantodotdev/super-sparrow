import { LabelDot } from "./label-dot";
import type { Label } from "../lib/types";

type LabelPickerProps = {
  options: Label[];
  selected: string[];
  onChange: (labelIds: string[]) => void;
};

/** Lets the user toggle labels on and off; selected and onChange use label IDs. */
export function LabelPicker({ options, selected, onChange }: LabelPickerProps) {
  const toggle = (label: string) =>
    onChange(
      selected.includes(label) ? selected.filter((item) => item !== label) : [...selected, label],
    );

  return (
    <fieldset className="mx-0 my-[16px] border-0 p-0 [&_legend]:mb-[8px] [&_legend]:font-medium [&_legend]:text-[#ccd5e1]">
      <legend>Labels</legend>
      <div className="flex max-h-[150px] flex-wrap gap-[6px] overflow-y-auto">
        {options.map(({ id, name, color }) => (
          <button
            type="button"
            key={id}
            className={`inline-flex items-center gap-2 rounded-full border border-[#303642] bg-[#101216] px-[9px] py-[5px] text-[12px] text-[#a9b6c8] hover:bg-[#20242c] [&.is-selected]:border-[#6685ee] [&.is-selected]:bg-[#202b45] [&.is-selected]:text-[#eaf0ff] ${selected.includes(id) ? "is-selected" : ""}`}
            aria-pressed={selected.includes(id)}
            onClick={() => toggle(id)}
          >
            <LabelDot color={color} /> {name}
          </button>
        ))}
      </div>
    </fieldset>
  );
}
