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
    <aside className="sticky top-0 left-0 z-1 flex h-dvh w-[274px] shrink-0 basis-[274px] flex-col self-start border-r border-[#20242b] bg-[#07080a] px-[13px] pt-[24px] pb-4 max-[700px]:w-[66px] max-[700px]:basis-[66px] max-[700px]:px-[8px] max-[700px]:py-[20px]">
      <div className="flex items-center gap-[12px] px-[13px] text-[17px] font-[650] whitespace-nowrap max-[700px]:px-[5px] max-[700px]:text-[0px]">
        <img src="/logo.png" className="h-[40px] w-[40px] flex-none" alt="" /> Super Sparrow
      </div>
      <nav className="mt-[32px]" aria-label="Main navigation">
        <NavItem to="/issues" icon="circle-user-round" label="My issues" />
        <NavItem to="/activity" icon="activity" label="Activity" active={section === "activity"} />
        <div className="mx-[15px] mt-[25px] mb-[12px] text-[13px] text-[#9ba9be] max-[700px]:hidden">
          Projects{" "}
          <button
            className="float-right border-0 bg-transparent text-[#9ba9be]"
            aria-label="Add project"
            onClick={() => notify("Project creation is a preview")}
          >
            <Icon name="plus" />
          </button>
        </div>
        <button
          className="flex h-[34px] w-full items-center gap-[12px] rounded-[6px] border-0 bg-transparent px-[15px] text-left transition-colors duration-160 hover:bg-[#20252d] max-[700px]:p-[10px] max-[700px]:text-[0px]"
          onClick={() => setCollapsed(!collapsed)}
        >
          <Icon
            name="folder"
            className="project-logo text-[20px] text-[#7890ff] max-[700px]:text-[20px]"
          />{" "}
          <strong>Sparrow</strong>
          <Icon
            name={collapsed ? "chevron-right" : "chevron-down"}
            className="ml-auto text-[#a5b1c2] max-[700px]:hidden"
          />
        </button>
        {!collapsed && (
          <div className="ml-[16px] max-[700px]:ml-0 [&_a]:pl-[20px] max-[700px]:[&_a]:p-0">
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
      <div className="mt-auto">
        <Link
          to="/signin"
          className="text-button mx-[12px] mt-0 mb-[8px] border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
        >
          Sign-in preview
        </Link>
        <NavItem to="/settings" icon="settings" label="Settings" active={section === "settings"} />
      </div>
    </aside>
  );
}
