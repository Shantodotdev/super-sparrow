import { createFileRoute } from "@tanstack/react-router";
import { SettingsHeader } from "../components/settings-header";
import { useToast } from "../components/toast";

export const Route = createFileRoute("/settings_/sessions")({
  staticData: { section: "settings" },
  component: SessionsPage,
});

/** The browsers and devices signed in to the current account. */
function SessionsPage() {
  const { notify } = useToast();

  return (
    <>
      <SettingsHeader title="Signed-in devices" description="Manage your active sessions." />
      <div className="settings-content">
        <section className="panel">
          <h2>Signed-in devices</h2>
          <div className="data-row">
            <div>
              <strong>Current browser</strong> <span className="badge">Current</span>
              <div className="muted small">Last seen just now</div>
            </div>
            <button className="button" onClick={() => notify("This is a frontend preview")}>
              Sign out
            </button>
          </div>
        </section>
      </div>
    </>
  );
}
