import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiWrite } from "../lib/api";
import type { Actor } from "../lib/types";
import { actorsQuery, meQuery } from "../queries/project";
import { Avatar } from "./avatar";
import { Icon } from "./icon";
import { Input } from "./input";
import { Modal } from "./modal";

type AvatarOption = { id: string; name: string; url: string };
type ProfileModalProps = { actor: Actor; open: boolean; onClose: () => void };

/** Edits the signed-in person's name and avatar from the server's built-in catalog. */
export function ProfileModal({ actor, open, onClose }: ProfileModalProps) {
  const client = useQueryClient();
  const [name, setName] = useState(actor.display_name);
  const [avatarID, setAvatarID] = useState(actor.avatar_id ?? "");
  const avatars = useQuery({
    queryKey: ["profile-avatars"],
    queryFn: async () => (await apiGet<{ avatars: AvatarOption[] }>("/api/avatars")).avatars,
    enabled: open,
  });
  const save = useMutation({
    mutationFn: () =>
      apiWrite<{ actor: Actor }>("/api/me", "PATCH", { display_name: name, avatar_id: avatarID }),
    onSuccess: ({ actor: updated }) => {
      client.setQueryData(meQuery.queryKey, updated);
      client.setQueryData(actorsQuery.queryKey, (current) =>
        current?.map((item) => (item.id === updated.id ? updated : item)),
      );
      onClose();
      void client.invalidateQueries();
    },
  });

  useEffect(() => {
    if (!open) return;
    setName(actor.display_name);
    setAvatarID(actor.avatar_id ?? "");
    save.reset();
  }, [open]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (name.trim() && !save.isPending) save.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={() => {
        if (!save.isPending) onClose();
      }}
      label="Edit profile"
      className="max-h-[80dvh] w-[min(100%,580px)] overflow-y-auto rounded-[12px] border border-[#303641] bg-[#12151a] p-6 shadow-[0_20px_70px_#000a]"
    >
      <form onSubmit={submit}>
        <h2>Edit profile</h2>
        <div className="mt-5 flex items-center gap-4">
          <div className="[&_.avatar]:size-16 [&_.avatar]:basis-16 [&_.avatar]:text-lg">
            <Avatar actor={{ ...actor, display_name: name, avatar_id: avatarID }} />
          </div>
          <label className="m-0 min-w-0 flex-1 gap-2">
            Name
            <Input
              autoFocus
              maxLength={100}
              required
              value={name}
              disabled={save.isPending}
              onChange={(event) => {
                setName(event.target.value);
                save.reset();
              }}
            />
          </label>
        </div>
        <fieldset className="mt-6 min-w-0 border-0 p-0">
          <legend className="mb-3 text-sm font-medium text-[#ccd5e1]">Avatar</legend>
          {avatars.isPending && <p className="text-sm text-[#95a6bd]">Loading avatars…</p>}
          {avatars.error && (
            <p role="alert" className="text-sm text-[#f2a9ae]">
              {avatars.error.message}
            </p>
          )}
          <div
            className="grid max-h-[42dvh] grid-cols-5 gap-2 overflow-y-auto p-1 sm:grid-cols-6"
            aria-label="Available avatars"
          >
            <button
              type="button"
              aria-label="Use initials"
              aria-pressed={!avatarID}
              disabled={save.isPending}
              className="aspect-square rounded-xl border border-[#303744] bg-[#101216] p-2 text-xs text-[#c7cfdb] transition-colors hover:bg-[#283140] focus-visible:border-[#6486ff] focus-visible:outline-none aria-pressed:border-[#6486ff] aria-pressed:bg-[#202b45]"
              onClick={() => {
                setAvatarID("");
                save.reset();
              }}
            >
              Initials
            </button>
            {avatars.data?.map((avatar) => (
              <button
                key={avatar.id}
                type="button"
                aria-label={`Select ${avatar.name}`}
                aria-pressed={avatarID === avatar.id}
                disabled={save.isPending}
                title={avatar.name}
                className="relative aspect-square overflow-hidden rounded-xl border border-[#303744] bg-[#101216] p-1 transition-colors hover:bg-[#283140] focus-visible:border-[#6486ff] focus-visible:outline-none disabled:opacity-50 aria-pressed:border-[#6486ff] aria-pressed:bg-[#202b45]"
                onClick={() => {
                  setAvatarID(avatar.id);
                  save.reset();
                }}
              >
                <img src={avatar.url} alt="" className="size-full rounded-lg object-cover" />
                {avatarID === avatar.id && (
                  <span className="absolute right-0.5 bottom-0.5 grid size-5 place-items-center rounded-full bg-[#2455eb] text-white">
                    <Icon name="check" className="size-3" />
                  </span>
                )}
              </button>
            ))}
          </div>
        </fieldset>
        {save.error && (
          <p role="alert" className="mt-3 text-sm text-[#f2a9ae]">
            {save.error.message}
          </p>
        )}
        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            disabled={save.isPending}
            className="rounded-full border border-[#303641] bg-[#14171c] px-4 py-2.5 font-[550] hover:bg-[#20242b] disabled:opacity-50"
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={save.isPending || !name.trim()}
            className="rounded-full border border-[#3264ff] bg-[#2455eb] px-4 py-2.5 font-[550] text-white hover:bg-[#3566f6] disabled:opacity-50"
          >
            Save changes
          </button>
        </div>
      </form>
    </Modal>
  );
}
