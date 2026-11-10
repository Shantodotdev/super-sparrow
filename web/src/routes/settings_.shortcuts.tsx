import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ShortcutRecorder } from "../components/shortcut-recorder";
import { SettingsHeader } from "../components/settings-header";
import { useKeyboardShortcuts } from "../components/keyboard-shortcuts-provider";
import { apiWrite } from "../lib/api";
import { shortcutDefinitions, type ShortcutID } from "../lib/shortcuts";
import { keyboardPreferencesQuery, type KeyboardPreferences } from "../queries/keyboard-shortcuts";
import { meQuery } from "../queries/project";

export const Route = createFileRoute("/settings_/shortcuts")({
  staticData: { section: "settings" },
  component: KeyboardShortcutsPage,
});

/** Splits the groups, in order, into two columns whose row counts are as close as possible. */
function balanceColumns(groups: string[], size: (group: string) => number): string[][] {
  const total = groups.reduce((sum, group) => sum + size(group), 0);
  let best = 0;
  let bestGap = Infinity;
  let left = 0;

  for (let split = 0; split <= groups.length; split++) {
    const gap = Math.abs(total - 2 * left);
    if (gap < bestGap) {
      best = split;
      bestGap = gap;
    }
    if (split < groups.length) left += size(groups[split]!);
  }

  return [groups.slice(0, best), groups.slice(best)];
}

/** Edits only the signed-in person's bindings, with conflict detection and individual resets. */
function KeyboardShortcutsPage() {
  const client = useQueryClient();
  const identity = useQuery(meQuery);
  const query = keyboardPreferencesQuery(identity.data?.id ?? "");
  const preferences = useQuery(query);
  const { bindings, desktopError } = useKeyboardShortcuts();
  const update = useMutation({
    mutationFn: ({ action, binding }: { action: ShortcutID; binding: string | null }) =>
      apiWrite<KeyboardPreferences>("/api/me/shortcuts", "PATCH", {
        action,
        binding,
        revision: preferences.data?.revision,
      }),
    onSuccess: (saved) => client.setQueryData(query.queryKey, saved),
    onError: () => {
      void client.invalidateQueries({ queryKey: query.queryKey });
    },
  });
  const ids = Object.keys(shortcutDefinitions) as ShortcutID[];
  const groups = [...new Set(ids.map((id) => shortcutDefinitions[id].group))];
  const columns = balanceColumns(
    groups,
    (group) => ids.filter((id) => shortcutDefinitions[id].group === group).length,
  );
  const problems = [
    preferences.error?.message,
    update.error?.message,
    desktopError &&
      `This desktop could not update its global shortcut: ${desktopError}. Choose another combination or cancel recording.`,
  ].filter(Boolean);

  return (
    <>
      <SettingsHeader />
      <main className="grid gap-[22px] px-[32px] py-[20px] max-[700px]:p-[16px]">
        {problems.map((message) => (
          <p
            key={message}
            role="alert"
            className="rounded-[8px] border border-[#5a2a33] bg-[#2a1519] px-4 py-[10px] text-[13px] text-[#f2a9ae]"
          >
            {message}
          </p>
        ))}

        {preferences.data && (
          <div className="grid grid-cols-2 items-start gap-[22px] max-[900px]:grid-cols-1">
            {columns.map((column, index) => (
              <div key={index} className="grid gap-[22px]">
                {column.map((group) => (
                  <section key={group} aria-label={group}>
                    <h3 className="m-0 mb-3 px-1 text-[13px] font-semibold tracking-[0.06em] text-[#8f9db2] uppercase">
                      {group}
                    </h3>
                    <ul className="grid gap-2">
                      {ids
                        .filter((id) => shortcutDefinitions[id].group === group)
                        .map((id) => (
                          <li
                            key={id}
                            className="group flex min-h-16 items-center justify-between gap-[16px] rounded-[8px] border border-[#222730] px-[16px] py-[6px] focus-within:bg-[#12151a] hover:bg-[#12151a]"
                          >
                            <label
                              htmlFor={`shortcut-${id}`}
                              className="m-0 min-w-0 text-[15px] text-[#d3dbe8]"
                            >
                              {shortcutDefinitions[id].label}
                            </label>
                            <ShortcutRecorder
                              id={id}
                              binding={bindings[id]}
                              customized={id in preferences.data.overrides}
                              disabled={update.isPending}
                              onChange={(binding) => update.mutate({ action: id, binding })}
                            />
                          </li>
                        ))}
                    </ul>
                  </section>
                ))}
              </div>
            ))}
          </div>
        )}
      </main>
    </>
  );
}
