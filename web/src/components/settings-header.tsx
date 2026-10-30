import { Tabs } from "./tabs";
import { Link } from "@tanstack/react-router";
import { Icon } from "./icon";

type SettingsHeaderProps = { title: string; description: string };

/** The heading shared by the settings pages, with a tab link for each of them. */
export function SettingsHeader({ title, description }: SettingsHeaderProps) {
  return (
    <header className="border-b border-b-[#20242b] px-[32px] pt-[29px] pb-0 max-[700px]:px-[20px] max-[700px]:pt-[24px] max-[700px]:pb-0 [&_>_p]:mx-0 [&_>_p]:mt-[10px] [&_>_p]:mb-[30px] [&_>_p]:text-[16px]">
      <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
        Settings
      </div>
      <h1>{title}</h1>
      <p className="muted text-[#a2aec2]">{description}</p>
      <Tabs className="mt-[20px]">
        <Link
          to="/settings"
          activeOptions={{ exact: true }}
          activeProps={{ className: "selected" }}
        >
          <Icon name="users" /> &nbsp; People and agents
        </Link>
        <Link to="/settings/sessions" activeProps={{ className: "selected" }}>
          <Icon name="monitor" /> &nbsp; Signed-in devices
        </Link>
      </Tabs>
    </header>
  );
}
