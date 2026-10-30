import type { ComponentPropsWithRef } from "react";

/** A native input with a smoothly changing focus border and no outline. */
export function Input({ className = "", ...props }: ComponentPropsWithRef<"input">) {
  return <input {...props} className={`text-field focus:border-[#6486ff] ${className}`} />;
}

/** A native multiline field using the same focus treatment as Input. */
export function Textarea({ className = "", ...props }: ComponentPropsWithRef<"textarea">) {
  return (
    <textarea {...props} className={`text-field text-area focus:border-[#6486ff] ${className}`} />
  );
}
