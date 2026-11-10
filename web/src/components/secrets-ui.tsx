import type { ReactNode } from "react";

export const buttonClass =
  "rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b] disabled:opacity-50";
export const primaryClass =
  "rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6] disabled:opacity-50";
export const dangerClass =
  "rounded-full border border-[#a33a48] bg-[#7d1f2c] px-[15px] py-[10px] font-[550] text-white hover:bg-[#962636] disabled:opacity-50";
export const smallButtonClass =
  "inline-flex items-center justify-center rounded-full border border-[#303641] bg-[#14171c] px-3 py-1.5 text-[12px] font-[550] whitespace-nowrap text-[#d6dfed] transition-colors hover:border-[#484f5d] hover:bg-[#20242b] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
export const smallDangerClass =
  "inline-flex items-center justify-center rounded-full border border-[#713440] bg-[#30191f] px-3 py-1.5 text-[12px] font-[550] whitespace-nowrap text-[#f2a9ae] transition-colors hover:border-[#a33a48] hover:bg-[#4a222b] focus-visible:border-[#f2a9ae] focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50";
export const dialogClass =
  "max-h-[82dvh] w-[min(100%,560px)] overflow-y-auto rounded-[10px] border border-[#303641] bg-[#12151a] p-[24px] shadow-[0_20px_70px_#000a] [&_h2]:mb-[12px] [&_p]:mb-[16px]";

const badgeTones = {
  neutral: "border-[#2b313a] bg-[#191c22] text-[#adb9c9]",
  blue: "border-[#2f4a99] bg-[#16213f] text-[#9db6ff]",
  green: "border-[#1f6a4f] bg-[#10261f] text-[#65dca4]",
  amber: "border-[#6b5420] bg-[#2a2210] text-[#f0cf75]",
  red: "border-[#713440] bg-[#30191f] text-[#f2a9ae]",
} as const;

/** A small rounded label, such as "Required" or "Your override". */
export function Badge({
  tone = "neutral",
  children,
}: {
  tone?: keyof typeof badgeTones;
  children: ReactNode;
}) {
  return (
    <span
      className={`inline-block rounded-full border px-2 py-[3px] text-[12px] whitespace-nowrap ${badgeTones[tone]}`}
    >
      {children}
    </span>
  );
}

/** A bordered panel with a title, a description, and an optional action on the right. */
export function Panel({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]">
      <div className="mb-[20px] flex items-start justify-between gap-[20px] max-[700px]:flex-wrap [&_p]:mt-[3px]">
        <div>
          <h2>{title}</h2>
          {description && <p className="muted text-[#a2aec2]">{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

/** A red inline error message. */
export function ErrorText({ children }: { children: ReactNode }) {
  return (
    <p role="alert" className="text-sm text-[#ffb1b1]">
      {children}
    </p>
  );
}
