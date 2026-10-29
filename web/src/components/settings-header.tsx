import { Link } from "@tanstack/react-router";
import { Icon } from "./icon";

type SettingsHeaderProps = { title: string; description: string };

/** The heading shared by the settings pages, with a tab link for each of them. */
export function SettingsHeader({ title, description }: SettingsHeaderProps) {
  return (
    <header className="page-header">
      <div className="breadcrumb">Settings</div>
      <h1>{title}</h1>
      <p className="muted">{description}</p>
      <div className="tabs">
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
      </div>
    </header>
  );
}
