import { Link, type LinkProps } from "@tanstack/react-router";
import { Icon, type IconName } from "./icon";

type NavItemProps = {
  to: LinkProps["to"];
  icon: IconName;
  label: string;
  active?: boolean;
};

/** A sidebar link; `active` marks the section the current route belongs to. */
export function NavItem({ to, icon, label, active = false }: NavItemProps) {
  return (
    // An empty activeProps turns off the Link's own "active" class; the section decides instead.
    <Link to={to} activeProps={{}} className={`nav-item ${active ? "active" : ""}`}>
      <Icon name={icon} className="nav-icon" />
      {label}
    </Link>
  );
}
