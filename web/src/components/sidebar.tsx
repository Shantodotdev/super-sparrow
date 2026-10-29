import { useState } from "react";
import { Link, useMatches } from "@tanstack/react-router";
import { Icon } from "./icon";
import { NavItem } from "./nav-item";
import { useToast } from "./toast";

/** The main navigation. The highlighted item comes from the `section` in the route's static data. */
export function Sidebar() {
  const section = useMatches({ select: (matches) => matches.at(-1)?.staticData.section });
  const { notify } = useToast();
  const [collapsed, setCollapsed] = useState(false);

  return (
    <aside className="sidebar">
      <div className="brand">
        <img src="/logo.png" className="brand-mark" alt="" /> Super Sparrow
      </div>
      <nav className="sidebar-nav" aria-label="Main navigation">
        <NavItem to="/issues" icon="circle-user-round" label="My issues" />
        <NavItem to="/activity" icon="activity" label="Activity" active={section === "activity"} />
        <div className="nav-heading">
          Projects{" "}
          <button
            className="nav-add"
            aria-label="Add project"
            onClick={() => notify("Project creation is a preview")}
          >
            <Icon name="plus" />
          </button>
        </div>
        <button className="project-button" onClick={() => setCollapsed(!collapsed)}>
          <Icon name="folder" className="project-logo" /> <strong>Sparrow</strong>
          <Icon name={collapsed ? "chevron-right" : "chevron-down"} className="chevron" />
        </button>
        {!collapsed && (
          <div className="project-nav">
            <NavItem to="/issues" icon="list-todo" label="Issues" active={section === "issues"} />
            <NavItem
              to="/milestones"
              icon="target"
              label="Milestones"
              active={section === "milestones"}
            />
            <NavItem
              to="/findings"
              icon="list-filter"
              label="Open findings"
              active={section === "findings"}
            />
          </div>
        )}
      </nav>
      <div className="sidebar-bottom">
        <Link to="/signin" className="text-button signout-preview">
          Sign-in preview
        </Link>
        <NavItem to="/settings" icon="settings" label="Settings" active={section === "settings"} />
      </div>
    </aside>
  );
}
