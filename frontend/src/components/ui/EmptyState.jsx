export function EmptyState({ title, description }) {
  return (
    <div className="rounded border border-dashed border-slate-300 p-6 text-center text-sm text-slate-500">
      <div className="font-medium text-slate-700">{title}</div>
      <div className="mt-1">{description}</div>
    </div>
  );
}
