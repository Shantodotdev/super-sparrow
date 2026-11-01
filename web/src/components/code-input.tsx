import { Input } from "./input";
import { useRef, type ClipboardEvent, type KeyboardEvent } from "react";

const codeLength = 6;

/** Returns an empty code with one slot per digit. */
export function emptyCode(): string[] {
  return Array.from({ length: codeLength }, () => "");
}

type CodeInputProps = {
  digits: string[];
  onChange: (digits: string[]) => void;
  disabled?: boolean;
};

/** Six single-digit boxes for a one-time code. Typing advances focus, Backspace steps back, and pasting fills every box. */
export function CodeInput({ digits, onChange, disabled = false }: CodeInputProps) {
  const inputs = useRef<(HTMLInputElement | null)[]>([]);

  const write = (start: number, chars: string) => {
    const next = [...digits];
    const entered = chars.replace(/\D/g, "").slice(0, codeLength - start);
    [...entered].forEach((digit, offset) => {
      next[start + offset] = digit;
    });
    onChange(next);
    inputs.current[Math.min(start + entered.length, codeLength - 1)]?.focus();
  };

  const erase = (index: number) => {
    const target = digits[index] ? index : Math.max(index - 1, 0);
    const next = [...digits];
    next[target] = "";
    onChange(next);
    inputs.current[target]?.focus();
  };

  const handleChange = (index: number, value: string) => {
    const digitsOnly = value.replace(/\D/g, "");
    // Several digits at once come from browser autofill, which targets the first box.
    if (digitsOnly.length > 1) {
      write(0, digitsOnly);
      return;
    }
    write(index, digitsOnly);
  };

  const handleKeyDown = (index: number, event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Backspace") {
      event.preventDefault();
      erase(index);
    } else if (event.key === "ArrowLeft") {
      inputs.current[index - 1]?.focus();
    } else if (event.key === "ArrowRight") {
      inputs.current[index + 1]?.focus();
    }
  };

  const handlePaste = (event: ClipboardEvent<HTMLInputElement>) => {
    event.preventDefault();
    write(0, event.clipboardData.getData("text"));
  };

  return (
    <div className="flex gap-[10px]">
      {digits.map((digit, index) => (
        <Input
          key={index}
          ref={(element) => {
            inputs.current[index] = element;
          }}
          aria-label={`Code digit ${index + 1}`}
          autoComplete={index === 0 ? "one-time-code" : "off"}
          autoFocus={index === 0}
          inputMode="numeric"
          disabled={disabled}
          value={digit}
          onChange={(event) => handleChange(index, event.target.value)}
          onKeyDown={(event) => handleKeyDown(index, event)}
          onPaste={handlePaste}
          onFocus={(event) => event.target.select()}
          className="h-[56px] w-[48px] p-0 text-center text-[22px] font-[600] rounded-[12px]"
        />
      ))}
    </div>
  );
}
