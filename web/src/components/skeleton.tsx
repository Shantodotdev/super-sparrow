/** A pulsing dark placeholder block used inside loading skeletons. */
function Bone({ className }: { className: string }) {
  return (
    <div
      aria-hidden="true"
      className={`animate-pulse rounded bg-[#181c24] motion-reduce:animate-none ${className}`}
    />
  );
}

/** Full-frame dark skeleton shown while initial authentication or project metadata loads. */
export function AppShellSkeleton() {
  return (
    <div
      className="flex min-h-screen bg-[#08090b]"
      role="status"
      aria-label="Loading Super Sparrow"
    >
      <aside className="sticky top-0 left-0 flex h-dvh w-[274px] shrink-0 basis-[274px] flex-col border-r border-[#20242b] bg-[#07080a] px-[13px] pt-[24px] pb-4 max-[700px]:w-[66px] max-[700px]:basis-[66px] max-[700px]:px-[8px]">
        <div className="flex items-center justify-center">
          <Bone className="size-16 rounded-2xl max-[700px]:size-10" />
        </div>
        <div className="mt-[32px] grid gap-2 px-2">
          <Bone className="h-8 w-full rounded-[6px]" />
          <Bone className="h-8 w-4/5 rounded-[6px]" />
        </div>
        <div className="mt-7 grid gap-2 px-2">
          <Bone className="h-4 w-20 max-[700px]:hidden" />
          <Bone className="h-8 w-full rounded-[6px]" />
          <Bone className="ml-4 h-7 w-4/5 rounded-[6px] max-[700px]:hidden" />
          <Bone className="ml-4 h-7 w-3/4 rounded-[6px] max-[700px]:hidden" />
          <Bone className="ml-4 h-7 w-2/3 rounded-[6px] max-[700px]:hidden" />
        </div>
        <div className="mt-auto px-2">
          <Bone className="h-8 w-full rounded-[6px]" />
        </div>
      </aside>
      <main className="min-w-0 flex-1 pt-6">
        <div className="mb-5 flex items-center gap-3 px-[26px] max-[700px]:px-4">
          <Bone className="h-10 w-[260px] rounded-full" />
          <Bone className="h-9 w-24 rounded-full" />
          <Bone className="h-9 w-24 rounded-full max-[700px]:hidden" />
          <Bone className="ml-auto h-9 w-28 rounded-full" />
        </div>
        <IssueListSkeleton />
      </main>
    </div>
  );
}

/** Grouped issue list placeholders matching the active status sections. */
export function IssueListSkeleton() {
  const groups = [3, 4, 2];

  return (
    <div
      className="grid gap-3 px-[26px] pt-1 pb-[50px] max-[700px]:px-4"
      role="status"
      aria-label="Loading issues"
    >
      {groups.map((rows, groupIndex) => (
        <section
          key={groupIndex}
          className="overflow-hidden rounded-[10px] border border-[#1d222b]"
        >
          <div className="flex items-center gap-3 bg-[#151820] px-4 py-3">
            <Bone className="size-4 rounded-full" />
            <Bone className="h-4 w-24" />
            <Bone className="h-5 w-7 rounded-[5px]" />
          </div>
          <div className="divide-y divide-[#1b2028] bg-[#0b0d11]">
            {Array.from({ length: rows }).map((_, rowIndex) => (
              <div key={rowIndex} className="flex items-center gap-4 px-4 py-3.5">
                <Bone className="size-4 shrink-0 rounded" />
                <Bone className="h-4 w-16 shrink-0" />
                <Bone className="h-4 max-w-120 flex-1" />
                <Bone className="ml-auto h-6 w-20 rounded-full max-[700px]:hidden" />
                <Bone className="size-6 shrink-0 rounded-full" />
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

/** Kanban column placeholders matching the board layout. */
export function IssueBoardSkeleton() {
  const columns = [2, 3, 2, 2];

  return (
    <div
      className="issue-board flex min-h-0 flex-1 items-stretch gap-4 overflow-x-auto overflow-y-hidden px-[26px] pt-[4px] pb-[32px] max-[700px]:px-4"
      role="status"
      aria-label="Loading board"
    >
      {columns.map((cards, columnIndex) => (
        <section
          key={columnIndex}
          className="flex min-h-0 flex-[0_0_270px] flex-col rounded-[10px] border border-[#222831] bg-[#0d0f12] p-[12px]"
        >
          <div className="flex items-center gap-2 px-0.5 pt-0.5 pb-4">
            <Bone className="size-4 rounded-full" />
            <Bone className="h-4 w-24" />
            <Bone className="h-5 w-7 rounded-[5px]" />
          </div>
          <div className="grid gap-[9px]">
            {Array.from({ length: cards }).map((_, cardIndex) => (
              <div
                key={cardIndex}
                className="grid gap-3 rounded-[8px] border border-[#252b35] bg-[#12151a] p-[13px]"
              >
                <Bone className="h-3.5 w-14" />
                <Bone className="h-4 w-full" />
                <Bone className="h-4 w-3/4" />
                <div className="flex items-center justify-between pt-1">
                  <Bone className="size-6 rounded-full" />
                  <Bone className="h-3.5 w-12" />
                </div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

/** Two-column issue detail skeleton matching the plan body and properties sidebar. */
export function IssueDetailSkeleton() {
  return (
    <div
      className="grid min-h-screen grid-cols-[minmax(0,1fr)_440px] max-[1100px]:grid-cols-[minmax(0,1fr)_360px] max-[700px]:block"
      role="status"
      aria-label="Loading issue"
    >
      <div className="px-[32px] pt-[27px] pb-8 max-[700px]:px-[20px]">
        <Bone className="h-9 w-3/4 max-w-[520px] rounded-md" />
        <div className="mt-4 flex flex-wrap items-center gap-4">
          <Bone className="h-5 w-20" />
          <Bone className="h-7 w-24 rounded-full" />
          <Bone className="h-5 w-32" />
        </div>
        <div className="mt-[27px] flex gap-5 border-b border-b-[#252a32] pb-3">
          <Bone className="h-5 w-14" />
          <Bone className="h-5 w-20" />
          <Bone className="h-5 w-24" />
          <Bone className="h-5 w-20" />
          <Bone className="h-5 w-20" />
        </div>
        <div className="mt-7 grid gap-3">
          <Bone className="h-4 w-full" />
          <Bone className="h-4 w-11/12" />
          <Bone className="h-4 w-4/5" />
          <Bone className="mt-3 h-4 w-3/4" />
          <Bone className="h-4 w-5/6" />
          <Bone className="h-4 w-2/3" />
        </div>
      </div>
      <aside className="sticky top-0 h-dvh self-start border-l border-l-[#242932] p-[24px] max-[700px]:static max-[700px]:h-auto max-[700px]:border-t max-[700px]:border-l-0 max-[700px]:border-t-[#252a32]">
        <div className="grid gap-5">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="grid gap-2">
              <Bone className="h-3.5 w-20" />
              <Bone className="h-9 w-full rounded-full" />
            </div>
          ))}
        </div>
      </aside>
    </div>
  );
}

/** Generic content skeleton for route transitions without a specialized placeholder. */
export function PageContentSkeleton() {
  return (
    <div
      className="grid gap-6 px-[32px] py-8 max-[700px]:px-4"
      role="status"
      aria-label="Loading page"
    >
      <div className="grid gap-2">
        <Bone className="h-8 w-52 rounded-md" />
        <Bone className="h-4 w-80 max-w-full" />
      </div>
      <div className="grid gap-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <Bone key={index} className="h-14 w-full rounded-[8px]" />
        ))}
      </div>
    </div>
  );
}
