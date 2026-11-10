import { useEffect, useRef, useState } from "react";
import { recordChord, type ShortcutID } from "../lib/shortcuts";
import { useKeyboardShortcuts } from "./keyboard-shortcuts-provider";

const actionClass =
  "rounded-[6px] border border-[#2b313b] bg-[#14171c] px-2.5 py-1 text-[12px] font-[550] text-[#c3cddd] transition-colors hover:border-[#484f5d] hover:bg-[#1c2027] hover:text-white focus-visible:border-[#6486ff] focus-visible:outline-none disabled:pointer-events-none disabled:opacity-40";

/** Shows a binding as one keycap per key, with "then" between the steps of a sequence. */
function Keycaps({ binding }: { binding: string }) {
  if (!binding) return <span className="text-[13px] text-[#6f7a8d]">Disabled</span>;

  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      {binding.split(" ").map((step, index) => (
        <span key={index} className="inline-flex items-center gap-1.5">
          {index > 0 && <span className="text-[11px] text-[#6f7a8d]">then</span>}
          {step.split("+").map((key, keyIndex) => (
            <kbd
              key={keyIndex}
              className="min-w-[24px] rounded-[5px] border border-[#2b313b] border-b-[#2b313b] bg-[#161a20] px-1.5 py-[3px] text-center font-sans text-[12px] font-[550] text-[#b9c4d6]"
            >
              {key}
            </kbd>
          ))}
        </span>
      ))}
    </span>
  );
}

type ShortcutRecorderProps = {
  id: ShortcutID;
  binding: string;
  customized: boolean;
  disabled: boolean;
  onChange: (binding: string | null) => void;
};

/** Records one chord or a two-step navigation sequence, restoring normal hotkeys on cancellation. */
export function ShortcutRecorder({
  id,
  binding,
  customized,
  disabled,
  onChange,
}: ShortcutRecorderProps) {
  const { recording, setRecording, recordingReady } = useKeyboardShortcuts();
  const surface = useRef<HTMLDivElement>(null);
  const [firstStep, setFirstStep] = useState("");
  const latestChange = useRef(onChange);
  latestChange.current = onChange;
  const active = recording === id;

  useEffect(
    () => () => setRecording((current) => (current === id ? null : current)),
    [id, setRecording],
  );

  useEffect(() => {
    if (!active) return;
    let first = "";
    let timer: ReturnType<typeof setTimeout> | undefined;
    const cancel = () => setRecording(null);
    const finish = (value: string) => {
      clearTimeout(timer);
      setRecording(null);
      latestChange.current(value);
    };
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Tab") {
        cancel();
        return;
      }
      event.preventDefault();
      event.stopImmediatePropagation();
      if (event.key === "Escape") {
        cancel();
        return;
      }
      if (!recordingReady || event.repeat || event.isComposing) return;
      const chord = recordChord(event);
      if (!chord) return;
      if (first) {
        finish(`${first} ${chord}`);
        return;
      }
      if (id.startsWith("go") || id === "up") {
        first = chord;
        setFirstStep(chord);
        timer = setTimeout(() => finish(chord), 1200);
      } else finish(chord);
    };
    window.addEventListener("keydown", handleKey, true);
    window.addEventListener("blur", cancel);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("keydown", handleKey, true);
      window.removeEventListener("blur", cancel);
    };
  }, [active, recordingReady, id, setRecording]);

  return (
    <div
      ref={surface}
      className="flex items-center gap-1"
      onBlur={(event) => {
        if (active && !event.currentTarget.contains(event.relatedTarget)) setRecording(null);
      }}
    >
      {!active && (
        <span className="mr-2 flex gap-1.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
          <button
            type="button"
            disabled={disabled || !customized}
            onClick={() => onChange(null)}
            className={actionClass}
          >
            Reset
          </button>
          <button
            type="button"
            disabled={disabled || !binding}
            onClick={() => onChange("")}
            className={`${actionClass} hover:border-[#a33a48]! hover:bg-[#3a1d24]! hover:text-[#f2a9ae]!`}
          >
            Disable
          </button>
        </span>
      )}
      <button
        type="button"
        id={`shortcut-${id}`}
        aria-pressed={active}
        data-recording={active ? (recordingReady ? "ready" : "preparing") : undefined}
        disabled={disabled}
        onClick={() => {
          setFirstStep("");
          setRecording(active ? null : id);
        }}
        className="inline-flex min-h-[32px] items-center justify-end rounded-[8px] border border-transparent px-2 text-[13px] transition-colors hover:bg-[#1a1e25] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 aria-pressed:border-[#6486ff] aria-pressed:bg-[#18233d]"
      >
        {active ? (
          <span className="text-[#aebbd6]">
            {!recordingReady ? "…" : firstStep ? `${firstStep} then…` : "Press keys"}
          </span>
        ) : (
          <Keycaps binding={binding} />
        )}
      </button>
      <span className="sr-only" role="status">
        {active
          ? firstStep
            ? `Press a second key, or wait to use ${firstStep}. Escape cancels.`
            : "Press your shortcut. Escape cancels."
          : ""}
      </span>
    </div>
  );
}
