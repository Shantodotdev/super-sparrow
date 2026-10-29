type PageHeaderProps = {
  crumb: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
};

export function PageHeader({ crumb, title, description, action }: PageHeaderProps) {
  return (
    <header className="view-header">
      <div className="breadcrumb">{crumb}</div>
      <div className="title-row">
        <div>
          <h1>{title}</h1>
          {description && <p className="muted header-description">{description}</p>}
        </div>
        {action}
      </div>
    </header>
  );
}
