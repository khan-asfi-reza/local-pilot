import { useEffect, useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';

const inputClass =
  'w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-zinc-600 placeholder:text-zinc-600';

// Combobox is a free-text autocomplete: the typed value filters `options`, and a
// match can be picked from the dropdown — but any text is allowed (so a name that
// is not in the list can still be submitted, e.g. to pull a new model). Closes on
// outside click, matching the app's ModelPicker pattern.
export function Combobox({ value, onChange, options = [], placeholder = '' }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const onDown = (e) => {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, []);

  const q = (value || '').toLowerCase();
  const filtered = options.filter((o) => o.toLowerCase().includes(q));

  return (
    <div className="relative" ref={ref}>
      <div className="relative">
        <input
          value={value}
          onChange={(e) => {
            onChange(e.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          placeholder={placeholder}
          className={inputClass}
        />
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          className="absolute right-1.5 top-1/2 -translate-y-1/2 p-1 text-zinc-500 hover:text-zinc-300"
          tabIndex={-1}
        >
          <ChevronDown size={16} />
        </button>
      </div>
      {open && filtered.length > 0 && (
        <div className="absolute z-20 mt-1 max-h-56 w-full overflow-auto rounded-lg border border-zinc-700 bg-zinc-850 py-1 shadow-2xl">
          {filtered.map((o) => (
            <button
              key={o}
              type="button"
              // mousedown (not click) so the pick lands before the input blur closes it
              onMouseDown={(e) => {
                e.preventDefault();
                onChange(o);
                setOpen(false);
              }}
              className="block w-full truncate px-3 py-1.5 text-left text-sm text-zinc-200 hover:bg-zinc-800"
            >
              {o}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
