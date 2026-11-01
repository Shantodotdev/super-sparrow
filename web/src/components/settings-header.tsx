import { Tabs } from "./tabs";
import { useToast } from "./toast";
import { apiWrite } from "../lib/api";
import { Link } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Icon } from "./icon";

/** Tab navigation shared by the settings pages, with a sign-out button on the right. */
export function SettingsHeader() {
  const client = useQueryClient();
  const { notify } = useToast();
  const signOut = useMutation({
    mutationFn: () => apiWrite("/api/auth/sign-out", "POST"),
    onSuccess: () => {
      client.clear();
      window.location.assign("/signin");
    },
    onError: (error) => notify(error.message),
  });

  return (
    <header className="flex items-center justify-between px-[32px] pt-[12px] max-[700px]:px-[20px]">
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
      <button
        type="button"
        disabled={signOut.isPending}
        onClick={() => signOut.mutate()}
        className="mb-[10px] rounded-full border border-[#713440] bg-[#30191f] px-[15px] py-[6px] text-[13px] font-[550] whitespace-nowrap text-[#f2a9ae] transition-colors hover:border-[#a33a48] hover:bg-[#4a222b] disabled:cursor-not-allowed disabled:opacity-50"
      >
        Sign out
      </button>
    </header>
  );
}
