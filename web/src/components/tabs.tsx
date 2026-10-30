import type { ReactNode } from "react";

type TabsProps = { children: ReactNode; className?: string };

/** Styles route links as tabs, with an underline on the selected link. */
export function Tabs({ children, className = "" }: TabsProps) {
  return (
    <div
      className={`flex gap-[26px] [&>a]:relative [&>a]:border-0 [&>a]:bg-transparent [&>a]:px-[5px] [&>a]:pt-0 [&>a]:pb-[16px] [&>a]:text-[#9eabbd] [&>a.selected]:text-[#f5f6fa] [&>a.selected]:after:absolute [&>a.selected]:after:inset-x-0 [&>a.selected]:after:bottom-0 [&>a.selected]:after:h-[3px] [&>a.selected]:after:rounded-t-[3px] [&>a.selected]:after:bg-[#426fff] [&>a.selected]:after:content-[''] ${className}`}
    >
      {children}
    </div>
  );
}
