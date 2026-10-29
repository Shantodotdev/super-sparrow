import { Select } from "./select";

type FilterProps = {
  name: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
};

/** A dropdown that narrows a list, with an "All" choice added in front of the options. */
export function Filter({ name, value, onChange, options }: FilterProps) {
  return (
    <Select
      variant="filter"
      label={name}
      value={value}
      onChange={onChange}
      options={["All", ...options].map((item) => ({ value: item, label: item }))}
    />
  );
}
