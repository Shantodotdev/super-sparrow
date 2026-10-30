import { Select } from "./select";

type FilterProps = {
  name: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
};

/** The value of a filter's "All" choice, which narrows nothing. */
export const all = "all";

/** A dropdown that narrows a list, with an "All" choice added in front of the options. */
export function Filter({ name, value, onChange, options }: FilterProps) {
  return (
    <Select
      variant="filter"
      label={name}
      value={value}
      onChange={onChange}
      options={[{ value: all, label: "All" }, ...options]}
    />
  );
}
