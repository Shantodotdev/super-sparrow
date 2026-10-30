import { Icon, type IconName } from "./icon";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

type SelectOption = {
  value: string;
  label: string;
  icon?: IconName;
  iconClassName?: string;
};

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

  const selectedOption = options[selected];
  const selectedIcon = selectedOption?.icon ?? icon;

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
      className={`custom-select relative min-w-0 ${variant === "filter" ? "flex-none" : ""}`}
      ref={container}
    >
      <button
        ref={trigger}
        type="button"
        role="combobox"
        className={`flex items-center justify-between border text-left transition-[border-color,background,box-shadow] duration-140 ease-[ease] focus-visible:border-[#404957] focus-visible:bg-[#1b2028] focus-visible:outline-none motion-reduce:transition-none ${
          variant === "chip"
            ? "min-h-[29px] w-auto gap-[6px] rounded-full border-[#36383e] px-[9px] py-[4px] text-[12px] hover:focus-visible:border-[#484b54] hover:focus-visible:bg-[#383b42] [&>svg:first-child]:size-4"
            : variant === "filter"
              ? "min-h-12 w-full gap-[8px] rounded-[10px] px-[10px] py-0 text-[13px]"
              : "min-h-12 w-full gap-4 rounded-[10px] px-[12px] py-0"
        } ${
          open
            ? `border-[#404957] bg-[#1b2028] outline-none ${variant === "chip" ? "hover:border-[#484b54] hover:bg-[#383b42]" : ""}`
            : variant === "chip"
              ? "bg-[#2a2c31] hover:border-[#484b54] hover:bg-[#383b42]"
              : "border-[#303642] bg-[#101216] hover:border-[#485264] hover:bg-[#171b22]"
        } text-[#d7dfeb]`}
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
        {selectedIcon && (
          <Icon
            name={selectedIcon}
            className={
              selectedOption?.iconClassName ?? (variant === "chip" ? "text-[#a9adb6]" : "")
            }
          />
        )}
        {variant === "filter" && <span className="text-[#e5e8ef]">{label}</span>}
        <span
          className={`custom-select-value truncate ${variant === "filter" ? "text-[#a9b6c8]" : ""}`}
        >
          {options[selected]?.label ?? value}
        </span>
        {variant !== "chip" && (
          <Icon
            name="chevron-down"
            className={`h-4 w-4 flex-none transition-[transform,color] duration-150 ease-[ease] motion-reduce:transition-none ${open ? "rotate-180 text-[#c2cdde]" : "text-[#8999ad]"}`}
          />
        )}
      </button>
      {open && (
        <div
          className="absolute top-[calc(100%+6px)] left-0 z-30 max-h-[260px] min-w-[max(100%,165px)] animate-[select-in_120ms_ease-out] overflow-y-auto rounded-[12px] border border-[#303744] bg-[#171b21] p-[5px] shadow-[0_12px_28px_#0009,0_2px_8px_#0006] motion-reduce:animate-none"
          id={`${id}-listbox`}
          role="listbox"
          aria-label={label}
        >
          {options.map((option, index) => (
            <div
              key={option.value}
              id={`${id}-option-${index}`}
              role="option"
              aria-selected={option.value === value}
              className={`flex min-h-[34px] cursor-pointer items-center justify-between gap-[18px] rounded-[5px] px-[9px] py-2 whitespace-nowrap aria-selected:text-[#f2f5fc] ${index === active ? "bg-[#283140] text-white" : "text-[#bfcbdc]"}`}
              onPointerMove={() => setActive(index)}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(index)}
            >
              <span className="flex items-center gap-2">
                {option.icon && <Icon name={option.icon} className={option.iconClassName} />}
                {option.label}
              </span>
              {option.value === value && (
                <span className="text-[#7896ff]" aria-hidden="true">
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
