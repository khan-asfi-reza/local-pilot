import { Button } from './button';

// ConfirmDialog is the app's in-place confirmation modal — a themed replacement
// for window.confirm. destructive tints the action red; otherwise it uses the
// brand gradient. Closes on backdrop click (treated as cancel).
export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel = 'Confirm',
  destructive = false,
  onConfirm,
  onCancel,
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onCancel} />
      <div className="relative w-full max-w-sm rounded-2xl border border-zinc-800 bg-[#141416] p-5 shadow-2xl">
        <h3 className="text-base font-semibold text-zinc-100">{title}</h3>
        {body && <p className="mt-1.5 text-sm leading-relaxed text-zinc-400">{body}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            className={
              destructive
                ? 'bg-red-600 text-white hover:bg-red-500'
                : 'bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow hover:brightness-110'
            }
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
