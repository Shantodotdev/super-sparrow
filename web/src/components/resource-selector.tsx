import { useEffect, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Input } from "./input";
import { Icon } from "./icon";
import { LabelDot } from "./label-dot";
import { labelColors } from "../lib/label-colors";

type ResourceOption = { id: string; name: string; color?: string };
type ResourceSelectorProps = {
  label: string;
  options: ResourceOption[];
  selected: string[];
  multiple?: boolean;
  colors?: boolean;
  onChange: (ids: string[]) => Promise<unknown>;
  onCreate: (name: string, color?: string) => Promise<string>;
};

/** Searches project resources, selects existing values, or creates a value before attaching it. */
export function ResourceSelector({
  label,
  options,
  selected,
  multiple,
  colors,
  onChange,
  onCreate,
}: ResourceSelectorProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [draft, setDraft] = useState<string | null>(null);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const name = search.trim();
  const matches = options.filter((option) =>
    option.name.toLowerCase().includes(name.toLowerCase()),
  );
  const exact = options.some((option) => option.name.toLowerCase() === name.toLowerCase());
  const change = useMutation({
    mutationFn: async ({ id, name, color }: { id?: string; name?: string; color?: string }) => {
      const chosen = id ?? (await onCreate(name!, color));
      if (id === undefined) {
        setDraft(null);
        setSearch(name!);
      }
      const ids = multiple
        ? selected.includes(chosen)
          ? selected.filter((value) => value !== chosen)
          : [...selected, chosen]
        : chosen
          ? [chosen]
          : [];
      await onChange(ids);
    },
    onSuccess: () => {
      setSearch("");
      setDraft(null);
      setOpen(false);
      trigger.current?.focus();
    },
  });

  useEffect(() => {
    if (!open) return;
    const dismiss = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [open]);

  return (
    <div
      ref={container}
      className="relative"
      onKeyDown={(event) => {
        if (event.key === "Escape" && open) {
          event.stopPropagation();
          if (draft) setDraft(null);
          else {
            setOpen(false);
            trigger.current?.focus();
          }
        }
      }}
    >
      <button
        ref={trigger}
        type="button"
        aria-label={label}
        aria-expanded={open}
        disabled={change.isPending}
        className="flex min-h-12 w-full items-center justify-between gap-2 rounded-[10px] border border-[#303642] bg-[#101216] px-3 text-left text-[#d7dfeb] transition-colors hover:border-[#485264] hover:bg-[#171b22] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:opacity-50"
        onClick={() => {
          setOpen(!open);
          setSearch("");
          setDraft(null);
          change.reset();
        }}
      >
        <span className="truncate">
          {selected.length
            ? multiple
              ? `${selected.length} selected`
              : (options.find((option) => option.id === selected[0])?.name ?? "Select…")
            : `Set ${label.toLowerCase()}`}
        </span>
        <Icon name="chevron-down" className="size-4 shrink-0 text-[#8999ad]" />
      </button>
      {open && (
        <div
          aria-label={`Choose ${label.toLowerCase()}`}
          className="absolute top-[calc(100%+6px)] right-0 z-30 w-full min-w-[230px] animate-[select-in_120ms_ease-out] rounded-[12px] border border-[#303744] bg-[#171b21] p-1.5 shadow-[0_12px_28px_#0009,0_2px_8px_#0006] motion-reduce:animate-none"
        >
          {draft ? (
            <div className="p-2">
              <div className="mb-3 flex items-center gap-2 text-sm text-[#c7cfdb]">
                <button
                  type="button"
                  aria-label="Back to labels"
                  disabled={change.isPending}
                  onClick={() => setDraft(null)}
                >
                  <Icon name="chevron-right" className="size-4 rotate-180" />
                </button>
                Choose a color <span className="ml-auto truncate text-[#95a6bd]">{draft}</span>
              </div>
              <div className="grid grid-cols-4 gap-2">
                {labelColors.map((color) => (
                  <button
                    autoFocus={color === labelColors[0]}
                    key={color.value}
                    type="button"
                    aria-label={color.name}
                    disabled={change.isPending}
                    className="grid place-items-center gap-1.5 rounded-lg border border-[#303744] bg-[#101216] px-1 py-2 text-xs text-[#bfcbdc] hover:bg-[#283140] disabled:opacity-50"
                    onClick={() => change.mutate({ name: draft, color: color.value })}
                  >
                    <span
                      className="size-5 rounded-full"
                      style={{ backgroundColor: color.value }}
                    />
                    {color.name}
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <>
              <Input
                autoFocus
                aria-label={`Search or create ${label.toLowerCase()}`}
                placeholder={`Search ${label.toLowerCase()}…`}
                maxLength={colors ? 50 : 200}
                className="h-9 border-0 bg-transparent px-2 text-sm"
                value={search}
                disabled={change.isPending}
                onChange={(event) => {
                  setSearch(event.target.value);
                  change.reset();
                }}
                onKeyDown={(event) => {
                  if (event.key !== "Enter" || change.isPending) return;
                  event.preventDefault();
                  const existing = options.find(
                    (option) => option.name.toLowerCase() === name.toLowerCase(),
                  );
                  if (existing) change.mutate({ id: existing.id });
                  else if (name) {
                    if (colors) setDraft(name);
                    else change.mutate({ name });
                  }
                }}
              />
              <div className="mt-1 max-h-[240px] overflow-y-auto">
                {!multiple && (
                  <button
                    type="button"
                    className="flex w-full rounded-md px-2 py-2 text-sm text-[#95a6bd] hover:bg-[#283140]"
                    disabled={change.isPending}
                    onClick={() => change.mutate({ id: "" })}
                  >
                    No {label.toLowerCase()}
                  </button>
                )}
                {matches.map((option) => (
                  <button
                    key={option.id}
                    type="button"
                    aria-pressed={selected.includes(option.id)}
                    disabled={change.isPending}
                    className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm text-[#bfcbdc] hover:bg-[#283140] disabled:opacity-50"
                    onClick={() => change.mutate({ id: option.id })}
                  >
                    {option.color && <LabelDot color={option.color} />}
                    <span className="min-w-0 flex-1 truncate">{option.name}</span>
                    {selected.includes(option.id) && (
                      <Icon name="check" className="size-4 text-[#7896ff]" />
                    )}
                  </button>
                ))}
                {name && !exact && (
                  <button
                    type="button"
                    disabled={change.isPending}
                    className="flex w-full items-center gap-2 rounded-md bg-[#283140] px-2 py-2 text-left text-sm text-[#f2f5fc] disabled:opacity-50"
                    onClick={() => (colors ? setDraft(name) : change.mutate({ name }))}
                  >
                    <Icon name="plus" className="size-4 shrink-0 text-[#7896ff]" />
                    <span className="truncate">
                      Create {label.toLowerCase()}: “{name}”
                    </span>
                  </button>
                )}
                {!matches.length && !name && (
                  <p className="px-2 py-2 text-sm text-[#95a6bd]">Type a name to create one.</p>
                )}
              </div>
            </>
          )}
          {change.error && (
            <p role="alert" className="px-2 py-2 text-xs text-[#f2a9ae]">
              {change.error.message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
