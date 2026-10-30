import { Link, type LinkProps } from "@tanstack/react-router";
import { Icon, type IconName } from "./icon";

type NavItemProps = {
  to: LinkProps["to"];
  params?: LinkProps["params"];
  icon: IconName;
  label: string;
  active?: boolean;
};

/** A sidebar link; `active` marks the section the current route belongs to. */
export function NavItem({ to, params, icon, label, active = false }: NavItemProps) {
  return (
    // An empty activeProps turns off the Link's own "active" class; the section decides instead.
    <Link
      to={to}
      params={params}
      activeProps={{}}
      className={`flex h-[34px] w-full items-center gap-[15px] rounded-lg border-0 px-[16px] text-left text-[#aab6c8] transition-[background-color,color] duration-160 hover:bg-[#12151a] hover:text-[#e2e7ef] max-[700px]:justify-center max-[700px]:px-0 max-[700px]:text-[0px] ${active ? "bg-[#1a1e25] text-[#f1f4fa] hover:bg-[#1a1e25] hover:text-[#f1f4fa]" : "bg-transparent"}`}
    >
      <Icon
        name={icon}
        className="w-[17px] text-center text-[19px] leading-none max-[700px]:text-[20px]"
      />
      {label}
    </Link>
  );
}
