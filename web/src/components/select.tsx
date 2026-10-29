import { Icon, type IconName } from "./icon";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

type SelectOption = { value: string; label: string };

type SelectProps = {
  label: string;
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  variant?: "filter" | "default" | "chip";
  icon?: IconName;
};

/** A controlled select with an accessible, keyboard-operable listbox. */
export function Select({
  label,
  value,
  options,
  onChange,
  variant = "default",
  icon,
}: SelectProps) {
  const id = useId();
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const selected = Math.max(
    0,
    options.findIndex((option) => option.value === value),
  );

  useEffect(() => {
    if (!open) return;
    const dismiss = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [open]);

  useEffect(() => {
    if (open)
      document.getElementById(`${id}-option-${active}`)?.scrollIntoView({ block: "nearest" });
  }, [active, id, open]);

  const choose = (index: number) => {
    const option = options[index];
    if (!option) return;
    onChange(option.value);
    setOpen(false);
    trigger.current?.focus();
  };

  const keyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "Escape" && open) {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      return;
    }
    if (event.key === "Tab") {
      setOpen(false);
      return;
    }
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      if (!open) {
        setActive(selected);
        setOpen(true);
        return;
      }
      setActive((current) =>
        event.key === "Home"
          ? 0
          : event.key === "End"
            ? options.length - 1
            : (current + (event.key === "ArrowDown" ? 1 : -1) + options.length) % options.length,
      );
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (open) choose(active);
      else {
        setActive(selected);
        setOpen(true);
      }
    }
  };

  return (
    <div
      className={`custom-select custom-select--${variant} ${open ? "is-open" : ""}`}
      ref={container}
    >
      <button
        ref={trigger}
        type="button"
        role="combobox"
        className="custom-select-trigger"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? `${id}-listbox` : undefined}
        aria-activedescendant={open ? `${id}-option-${active}` : undefined}
        onKeyDown={keyDown}
        onClick={() => {
          if (!open) setActive(selected);
          setOpen(!open);
        }}
      >
        {icon && <Icon name={icon} />}
        {variant === "filter" && <span className="custom-select-label">{label}</span>}
        <span className="custom-select-value">{options[selected]?.label ?? value}</span>
        <Icon name="chevron-down" className="custom-select-chevron" />
      </button>
      {open && (
        <div className="custom-select-menu" id={`${id}-listbox`} role="listbox" aria-label={label}>
          {options.map((option, index) => (
            <div
              key={option.value}
              id={`${id}-option-${index}`}
              role="option"
              aria-selected={option.value === value}
              className={`custom-select-option ${index === active ? "is-active" : ""}`}
              onPointerMove={() => setActive(index)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(index)}
            >
              <span>{option.label}</span>
              {option.value === value && (
                <span className="custom-select-check" aria-hidden="true">
                  <Icon name="check" />
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
