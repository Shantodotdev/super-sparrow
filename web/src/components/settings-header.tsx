import { Tabs } from "./tabs";
import { useToast } from "./toast";
import { apiWrite } from "../lib/api";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "./icon";
import { clearQueryCache } from "../queries/persistence";
import { meQuery } from "../queries/project";

/** Tab navigation shared by the settings pages, with a sign-out button on the right. */
export function SettingsHeader() {
  const client = useQueryClient();
  const identity = useQuery(meQuery);
  const { notify } = useToast();
  const signOut = useMutation({
    mutationFn: () => apiWrite("/api/auth/sign-out", "POST"),
    onSuccess: () => {
      clearQueryCache(client);
      window.location.assign("/signin");
    },
    onError: (error) => notify(error.message),
  });

  return (
    <header className="flex flex-wrap items-center justify-between gap-x-4 px-[32px] pt-[12px] max-[700px]:px-[20px]">
      <Tabs className="min-w-0 overflow-x-auto whitespace-nowrap">
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
        <Link to="/settings/cli" activeProps={{ className: "selected" }}>
          <Icon name="terminal" /> &nbsp; Connect CLI
        </Link>
        {identity.data?.role === "owner" && (
          <Link to="/settings/discord" activeProps={{ className: "selected" }}>
            <img
              src="/brands/discord.svg"
              alt=""
              className="inline-block size-[17px] align-[-3px]"
            />
            &nbsp; Discord
          </Link>
        )}
        {identity.data?.role === "owner" && (
          <Link to="/settings/github" activeProps={{ className: "selected" }}>
            <Icon name="github" /> &nbsp; GitHub
          </Link>
        )}
        <Link to="/settings/shortcuts" activeProps={{ className: "selected" }}>
          <Icon name="keyboard" /> &nbsp; Keyboard shortcuts
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
