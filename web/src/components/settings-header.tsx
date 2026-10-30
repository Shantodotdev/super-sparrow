import { Tabs } from "./tabs";
import { Link } from "@tanstack/react-router";
import { Icon } from "./icon";

/** Tab navigation shared by the settings pages. */
export function SettingsHeader() {
  return (
    <header className="border-b border-b-[#20242b] px-[32px] pt-[12px] max-[700px]:px-[20px]">
      <Tabs>
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
