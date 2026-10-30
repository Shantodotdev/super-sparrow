type PageHeaderProps = {
  crumb: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
};

export function PageHeader({ crumb, title, description, action }: PageHeaderProps) {
  return (
    <header className="px-[26px] pt-[24px] pb-0 max-[700px]:px-[16px] max-[700px]:pt-[22px] max-[700px]:pb-0 [&_.breadcrumb]:mb-[20px]">
      <div className="breadcrumb mb-[22px] text-[#a9b8cb] [&_a]:border-0 [&_a]:bg-none [&_a]:p-0 [&_a]:text-inherit [&_a:hover]:text-[#d2e0ff] [&_button]:border-0 [&_button]:bg-none [&_button]:p-0 [&_button]:text-inherit [&_button:hover]:text-[#d2e0ff]">
        {crumb}
      </div>
      <div className="flex items-start justify-between gap-[24px] max-[700px]:flex-wrap">
        <div>
          <h1>{title}</h1>
          {description && (
            <p className="muted mt-[10px] text-[16px] text-[#a2aec2]">{description}</p>
          )}
        </div>
        {action}
      </div>
    </header>
  );
}
