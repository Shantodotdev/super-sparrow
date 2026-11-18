import { useSuspenseQuery } from "@tanstack/react-query";
import { useState, type MouseEvent } from "react";
import { createPortal } from "react-dom";
import { issueStatusLabel, issueStatuses, statusIcon, statusIconColor } from "../lib/issue-status";
import type { Issue, Subissue } from "../lib/types";
import { labelsQuery } from "../queries/project";
import { subissuesQuery, useSaveSubissue, type SubissueChanges } from "../queries/subissues";
import { ContextMenuShell, Item, Parent } from "./issue-context-menu";
import { Icon } from "./icon";
import { LabelDot } from "./label-dot";
import { useToast } from "./toast";

type MenuTarget = { subissue: Subissue; x: number; y: number };

/** Opens a menu of a subissue's status and labels on right-click. */
export function useSubissueContextMenu(issue: Issue) {
  const [target, setTarget] = useState<MenuTarget>();

  const open = (subissue: Subissue) => (event: MouseEvent) => {
    event.preventDefault();
    setTarget({ subissue, x: event.clientX, y: event.clientY });
  };

  const menu = target
    ? createPortal(
        <SubissueContextMenu
          key={`${target.subissue.id}:${target.x}:${target.y}`}
          issue={issue}
          target={target}
          onClose={() => setTarget(undefined)}
        />,
        document.body,
      )
    : null;

  return { open, menu, activeId: target?.subissue.id };
}

/** Lists the properties of one subissue as pickers; each choice is saved at once. */
function SubissueContextMenu({
  issue,
  target,
  onClose,
}: {
  issue: Issue;
  target: MenuTarget;
  onClose: () => void;
}) {
  const { data: subissues } = useSuspenseQuery(subissuesQuery(issue.key));
  const { data: labels } = useSuspenseQuery(labelsQuery);
  const save = useSaveSubissue(issue.key);
  const { notify } = useToast();

  // Follow saved edits so checkmarks update while the menu stays open.
  const subissue = subissues.find((item) => item.id === target.subissue.id) ?? target.subissue;
  const projectLabels = labels.filter((label) => label.project_id === issue.project_id);

  return (
    <ContextMenuShell
      label={`Actions for ${subissue.key}`}
      x={target.x}
      y={target.y}
      onClose={onClose}
    >
      {({ onClose: close, openSub, setOpenSub }) => {
        const choose = (change: SubissueChanges, keepOpen = false) => {
          save.mutate(
            { id: subissue.id, ...change },
            { onError: (error) => notify(error.message) },
          );
          if (!keepOpen) close();
        };

        return (
          <>
            <Parent
              id="status"
              label="Status"
              icon={
                <Icon
                  name={statusIcon[subissue.status]}
                  className={statusIconColor[subissue.status]}
                />
              }
              openSub={openSub}
              setOpenSub={setOpenSub}
            >
              {issueStatuses.map((status) => (
                <Item
                  key={status}
                  label={issueStatusLabel[status]}
                  icon={<Icon name={statusIcon[status]} className={statusIconColor[status]} />}
                  checked={subissue.status === status}
                  onSelect={() => choose({ status })}
                />
              ))}
            </Parent>
            <Parent
              id="labels"
              label="Labels"
              icon={<Icon name="tag" className="text-[#95a6bd]" />}
              detail={subissue.label_ids.length ? String(subissue.label_ids.length) : undefined}
              openSub={openSub}
              setOpenSub={setOpenSub}
            >
              {projectLabels.length === 0 && <Item label="No labels" disabled />}
              {projectLabels.map((label) => {
                const checked = subissue.label_ids.includes(label.id);

                return (
                  <Item
                    key={label.id}
                    label={label.name}
                    icon={<LabelDot color={label.color} />}
                    checked={checked}
                    onSelect={() =>
                      choose(
                        {
                          label_ids: checked
                            ? subissue.label_ids.filter((id) => id !== label.id)
                            : [...subissue.label_ids, label.id],
                        },
                        true,
                      )
                    }
                  />
                );
              })}
            </Parent>
            <div role="separator" className="mx-1 my-1 h-px shrink-0 bg-[#2c323c]" />
            <Item
              label="Copy subissue ID"
              icon={<Icon name="copy" className="text-[#95a6bd]" />}
              onSelect={() => {
                void navigator.clipboard.writeText(subissue.key).then(
                  () => notify(`Copied ${subissue.key}`),
                  () => notify("Could not copy to the clipboard"),
                );
                close();
              }}
              onHover={() => setOpenSub(undefined)}
            />
          </>
        );
      }}
    </ContextMenuShell>
  );
}
