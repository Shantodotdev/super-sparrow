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
      <div className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        <section className="rounded-[8px] border border-[#222730] bg-[#0d0f12] p-[22px] max-[700px]:p-[16px]">
          <h2>Signed-in devices</h2>
          <div className="flex items-center justify-between border-t border-t-[#20252c] px-0 py-[16px]">
            <div>
              <strong>Current browser</strong>{" "}
              <span className="inline-block rounded-full border border-[#2b313a] bg-[#191c22] px-2 py-[4px] text-[12px] text-[#adb9c9] capitalize">
                Current
              </span>
              <div className="muted mt-[3px] text-[12px] text-[#a2aec2]">Last seen just now</div>
            </div>
            <button
              className="button rounded-full border border-[#303641] bg-[#14171c] px-[15px] py-[10px] font-[550] hover:bg-[#20242b]"
              onClick={() => notify("This is a frontend preview")}
            >
              Sign out
            </button>
          </div>
        </section>
      </div>
    </>
  );
}
