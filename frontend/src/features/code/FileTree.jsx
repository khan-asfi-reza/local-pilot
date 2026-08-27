import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  Folder,
  FolderOpen,
  FolderPlus,
  FolderSearch,
  FolderUp,
  FilePlus2,
  Home,
  RefreshCw,
} from 'lucide-react';
import { Button } from '../../components/ui/button';
import { code } from '../../lib/api';
import { cn } from '../../lib/utils';
import { FileIcon } from './fileIcons';

function DirRow({ name, path: dirPath, onClick }) {
  return (
    <button
      type="button"
      onClick={() => onClick(dirPath)}
      className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-[13px] text-zinc-300 hover:bg-zinc-800/60"
    >
      <Folder size={15} className="shrink-0 text-amber-400/70" />
      <span className="truncate">{name}</span>
    </button>
  );
}

// DirBrowser walks the machine's folders server-side and reports the folder the
// user is standing in. Shared by "Open folder" and the new-project pane. With
// `fill`, the list stretches to whatever height its parent gives it.
export function DirBrowser({ path, onPathChange, className, fill = false }) {
  const [parent, setParent] = useState(null);
  const [dirs, setDirs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [draft, setDraft] = useState('');

  const load = useCallback(
    async (p) => {
      setLoading(true);
      setError(null);
      try {
        const data = await code.browseDir(p);
        onPathChange(data.path);
        setDraft(data.path);
        setParent(data.parent);
        setDirs(data.dirs || []);
      } catch (e) {
        setError(String(e.message || e));
      }
      setLoading(false);
    },
    [onPathChange],
  );

  useEffect(() => {
    load('');
  }, [load]);

  return (
    <div className={cn('flex flex-col gap-2', fill && 'min-h-0 flex-1', className)}>
      <div className="flex shrink-0 items-center gap-1 rounded-lg border border-zinc-800 bg-[#0c0c0e] px-3 py-2">
        {parent !== null && (
          <button
            type="button"
            onClick={() => load(parent)}
            className="shrink-0 rounded p-1 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200"
            title="Go up"
          >
            <FolderUp size={15} />
          </button>
        )}
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              load(draft.trim());
            } else if (e.key === 'Escape') {
              setDraft(path);
            }
          }}
          spellCheck={false}
          placeholder="~"
          title="Type or paste a path, then press Enter"
          className="min-w-0 flex-1 bg-transparent text-[13px] text-zinc-200 outline-none placeholder:text-zinc-600"
        />
      </div>

      <div
        className={cn(
          'overflow-y-auto rounded-lg border border-zinc-800 bg-[#0c0c0e]',
          fill ? 'min-h-0 flex-1' : 'max-h-60',
        )}
      >
        {loading ? (
          <div className="px-3 py-4 text-xs text-zinc-600">Loading...</div>
        ) : error ? (
          <div className="px-3 py-4 text-xs text-red-400">{error}</div>
        ) : dirs.length === 0 ? (
          <div className="px-3 py-4 text-xs text-zinc-600">No subdirectories</div>
        ) : (
          dirs.map((d) => <DirRow key={d.path} name={d.name} path={d.path} onClick={load} />)
        )}
      </div>
    </div>
  );
}

// ContextMenu is the right-click menu, positioned at the pointer and clamped to
// the viewport. items: [{label, danger?, onSelect} | 'separator'].
function ContextMenu({ x, y, items, onClose }) {
  const ref = useRef(null);
  const [pos, setPos] = useState({ left: x, top: y });

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setPos({
      left: Math.min(x, window.innerWidth - rect.width - 8),
      top: Math.min(y, window.innerHeight - rect.height - 8),
    });
  }, [x, y]);

  useEffect(() => {
    const onDown = (e) => {
      if (!ref.current?.contains(e.target)) onClose();
    };
    const onKey = (e) => e.key === 'Escape' && onClose();
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    window.addEventListener('resize', onClose);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onClose);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      style={pos}
      className="fixed z-[60] min-w-[188px] rounded-xl border border-zinc-800 bg-[#18181b] p-1 shadow-2xl"
    >
      {items.map((item, i) =>
        item === 'separator' ? (
          <div key={i} className="my-1 border-t border-zinc-800" />
        ) : (
          <button
            key={i}
            type="button"
            onClick={() => {
              onClose();
              item.onSelect();
            }}
            className={cn(
              'flex w-full items-center justify-between gap-4 rounded-lg px-3 py-1.5 text-left text-[13px]',
              item.danger ? 'text-red-400 hover:bg-red-500/10' : 'text-zinc-200 hover:bg-zinc-800',
            )}
          >
            <span>{item.label}</span>
            {item.hint && <span className="text-[11px] text-zinc-600">{item.hint}</span>}
          </button>
        ),
      )}
    </div>
  );
}

// NameInput is the inline row used for both "new file/folder" and rename.
function NameInput({ initial = '', depth, icon, onSubmit, onCancel }) {
  const [value, setValue] = useState(initial);
  const ref = useRef(null);
  const done = useRef(false); // Enter then blur must not submit twice

  useEffect(() => {
    ref.current?.focus();
    const dot = initial.lastIndexOf('.');
    ref.current?.setSelectionRange(0, dot > 0 ? dot : initial.length);
  }, [initial]);

  const finish = (fn, arg) => {
    if (done.current) return;
    done.current = true;
    fn(arg);
  };

  return (
    <div
      className="flex items-center gap-1 py-0.5 pr-2"
      style={{ paddingLeft: `${depth * 16 + 22}px` }}
    >
      {icon}
      <input
        ref={ref}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() =>
          value.trim() && value.trim() !== initial ? finish(onSubmit, value.trim()) : finish(onCancel)
        }
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            if (value.trim()) finish(onSubmit, value.trim());
          } else if (e.key === 'Escape') {
            e.preventDefault();
            finish(onCancel);
          }
        }}
        className="min-w-0 flex-1 rounded border border-violet-500/60 bg-[#0c0c0e] px-1 py-0.5 text-[13px] text-zinc-100 outline-none"
      />
    </div>
  );
}

function TreeNode({ node, depth, ctx }) {
  const { expanded, toggle, onOpenFile, onContextMenu, renaming, onRename, onCancelEdit, pending, onCreate, activePath } = ctx;
  const isOpen = expanded.has(node.path);

  if (renaming === node.path) {
    return (
      <NameInput
        initial={node.name}
        depth={depth}
        icon={node.type === 'dir' ? <Folder size={15} className="shrink-0 text-amber-400/70" /> : <FileIcon name={node.name} size={15} />}
        onSubmit={(name) => onRename(node.path, name)}
        onCancel={onCancelEdit}
      />
    );
  }

  if (node.type === 'dir') {
    const adding = pending && pending.parent === node.path;
    return (
      <div>
        <button
          type="button"
          onClick={() => toggle(node.path)}
          onContextMenu={(e) => onContextMenu(e, node)}
          className="flex w-full items-center gap-1 rounded-lg px-2 py-1 text-left text-[13px] text-zinc-300 hover:bg-zinc-800/40"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
        >
          {isOpen ? (
            <ChevronDown size={14} className="shrink-0 text-zinc-500" />
          ) : (
            <ChevronRight size={14} className="shrink-0 text-zinc-500" />
          )}
          {isOpen ? (
            <FolderOpen size={15} className="shrink-0 text-amber-400/70" />
          ) : (
            <Folder size={15} className="shrink-0 text-amber-400/70" />
          )}
          <span className="truncate">{node.name}</span>
        </button>
        {(isOpen || adding) && (
          <>
            {adding && (
              <NameInput
                depth={depth + 1}
                icon={
                  pending.type === 'dir' ? (
                    <Folder size={15} className="shrink-0 text-amber-400/70" />
                  ) : (
                    <FilePlus2 size={15} className="shrink-0 text-zinc-500" />
                  )
                }
                onSubmit={(name) => onCreate(node.path, name, pending.type)}
                onCancel={onCancelEdit}
              />
            )}
            {isOpen &&
              node.children?.map((child) => (
                <TreeNode key={child.path} node={child} depth={depth + 1} ctx={ctx} />
              ))}
          </>
        )}
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={() => onOpenFile(node.path)}
      onContextMenu={(e) => onContextMenu(e, node)}
      className={cn(
        'flex w-full items-center gap-1 rounded-lg px-2 py-1 text-left text-[13px] hover:bg-zinc-800/40 hover:text-zinc-200',
        node.path === activePath ? 'bg-zinc-800/60 text-zinc-100' : 'text-zinc-400',
      )}
      style={{ paddingLeft: `${depth * 16 + 24}px` }}
    >
      <FileIcon name={node.name} size={15} />
      <span className="truncate">{node.name}</span>
    </button>
  );
}

// ConfirmDelete is the "are you sure" step before an irreversible delete.
function ConfirmDelete({ target, onConfirm, onCancel }) {
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm" onClick={onCancel}>
      <div
        className="w-full max-w-sm rounded-2xl border border-zinc-800 bg-[#141416] p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <p className="text-sm font-medium text-zinc-100">
          Delete {target.type === 'dir' ? 'folder' : 'file'} “{target.name}”?
        </p>
        <p className="mt-2 text-[13px] leading-relaxed text-zinc-400">
          {target.type === 'dir'
            ? 'The folder and everything inside it will be permanently deleted from disk.'
            : 'The file will be permanently deleted from disk.'}{' '}
          This cannot be undone.
        </p>
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={onConfirm}
            className="bg-red-600 hover:bg-red-500 focus-visible:ring-red-500"
          >
            Delete
          </Button>
        </div>
      </div>
    </div>
  );
}

const parentOf = (path) => {
  const i = path.lastIndexOf('/');
  return i < 0 ? '' : path.slice(0, i);
};
const joinPath = (dir, name) => (dir ? `${dir}/${name}` : name);

export function FileTree({
  root,
  projectName,
  tree,
  activePath,
  revealPath,
  onOpenFile,
  onCloseProject,
  onRefresh,
  onDeleted,
  onRenamed,
}) {
  const navigate = useNavigate();
  const [expanded, setExpanded] = useState(() => new Set());
  const [menu, setMenu] = useState(null); // { x, y, node }
  const [pending, setPending] = useState(null); // { parent, type }
  const [renaming, setRenaming] = useState(null); // path
  const [confirming, setConfirming] = useState(null); // node
  const [error, setError] = useState(null);
  const seeded = useRef(false);

  useEffect(() => {
    seeded.current = false;
    setExpanded(new Set());
    setPending(null);
    setRenaming(null);
  }, [root]);

  // Expand the top level once, the first time a project's tree arrives.
  useEffect(() => {
    if (seeded.current || !tree?.length) return;
    seeded.current = true;
    setExpanded(new Set(tree.filter((n) => n.type === 'dir').map((n) => n.path)));
  }, [tree]);

  // Reveal a path the agent just touched by expanding its ancestors.
  useEffect(() => {
    if (!revealPath) return;
    const parts = revealPath.split('/');
    parts.pop();
    if (!parts.length) return;
    setExpanded((prev) => {
      const next = new Set(prev);
      let acc = '';
      for (const part of parts) {
        acc = acc ? `${acc}/${part}` : part;
        next.add(acc);
      }
      return next;
    });
  }, [revealPath]);

  const toggle = useCallback((path) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const startCreate = useCallback((parent, type) => {
    setRenaming(null);
    setPending({ parent, type });
    if (parent) {
      setExpanded((prev) => new Set(prev).add(parent));
    }
  }, []);

  const cancelEdit = useCallback(() => {
    setPending(null);
    setRenaming(null);
  }, []);

  const create = useCallback(
    async (parent, name, type) => {
      setPending(null);
      const path = joinPath(parent, name);
      try {
        await code.createEntry(root, path, type);
        if (type === 'dir') setExpanded((prev) => new Set(prev).add(path));
        await onRefresh?.();
        if (type === 'file') onOpenFile?.(path);
      } catch (e) {
        setError(String(e.message || e));
      }
    },
    [root, onRefresh, onOpenFile],
  );

  const rename = useCallback(
    async (path, name) => {
      setRenaming(null);
      const next = joinPath(parentOf(path), name);
      if (next === path) return;
      try {
        await code.renameEntry(root, path, next);
        onRenamed?.(path, next);
        await onRefresh?.();
      } catch (e) {
        setError(String(e.message || e));
      }
    },
    [root, onRefresh, onRenamed],
  );

  const remove = useCallback(
    async (node) => {
      setConfirming(null);
      try {
        await code.deleteEntry(root, node.path);
        onDeleted?.(node.path);
        await onRefresh?.();
      } catch (e) {
        setError(String(e.message || e));
      }
    },
    [root, onRefresh, onDeleted],
  );

  const openMenu = useCallback((e, node) => {
    e.preventDefault();
    e.stopPropagation();
    setMenu({ x: e.clientX, y: e.clientY, node });
  }, []);

  const menuItems = useCallback(
    (node) => {
      const dir = !node ? '' : node.type === 'dir' ? node.path : parentOf(node.path);
      const items = [
        { label: 'New File', onSelect: () => startCreate(dir, 'file') },
        { label: 'New Folder', onSelect: () => startCreate(dir, 'dir') },
      ];
      if (node) {
        items.push('separator');
        items.push({ label: 'Rename', hint: 'F2', onSelect: () => setRenaming(node.path) });
        items.push({ label: 'Delete', danger: true, onSelect: () => setConfirming(node) });
        items.push('separator');
        items.push({
          label: 'Copy Path',
          onSelect: () => navigator.clipboard?.writeText(`${root}/${node.path}`).catch(() => {}),
        });
        items.push({
          label: 'Copy Relative Path',
          onSelect: () => navigator.clipboard?.writeText(node.path).catch(() => {}),
        });
      }
      return items;
    },
    [root, startCreate],
  );

  const ctx = {
    expanded,
    toggle,
    onOpenFile,
    onContextMenu: openMenu,
    renaming,
    onRename: rename,
    onCancelEdit: cancelEdit,
    pending,
    onCreate: create,
    activePath,
  };

  return (
    <div className="flex h-full w-[260px] shrink-0 flex-col border-r border-zinc-800 bg-[#101012]">
      <div className="flex items-center gap-0.5 px-3 py-2.5">
        <span className="mr-auto min-w-0 truncate text-sm font-medium text-zinc-300">
          {projectName || 'Files'}
        </span>
        <button
          type="button"
          onClick={() => navigate('/')}
          title="Home"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <Home size={14} />
        </button>
        <button
          type="button"
          onClick={() => startCreate('', 'file')}
          title="New file"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <FilePlus2 size={14} />
        </button>
        <button
          type="button"
          onClick={() => startCreate('', 'dir')}
          title="New folder"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <FolderPlus size={14} />
        </button>
        <button
          type="button"
          onClick={onRefresh}
          title="Refresh"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <RefreshCw size={14} />
        </button>
        <button
          type="button"
          onClick={() => setExpanded(new Set())}
          title="Collapse all"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <ChevronsDownUp size={14} />
        </button>
        <button
          type="button"
          onClick={onCloseProject}
          title="Close project — back to the project list"
          className="rounded-md p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          <FolderSearch size={14} />
        </button>
      </div>

      {error && (
        <div className="mx-2 mb-1 flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/10 px-2 py-1.5 text-[12px] text-red-300">
          <span className="min-w-0 flex-1 break-words">{error}</span>
          <button type="button" onClick={() => setError(null)} className="shrink-0 text-red-400/70 hover:text-red-300">
            ×
          </button>
        </div>
      )}

      {root && tree && (
        <div
          className="min-h-0 flex-1 overflow-y-auto px-1 py-1"
          onContextMenu={(e) => {
            if (e.target === e.currentTarget) openMenu(e, null);
          }}
        >
          {pending && pending.parent === '' && (
            <NameInput
              depth={0}
              icon={
                pending.type === 'dir' ? (
                  <Folder size={15} className="shrink-0 text-amber-400/70" />
                ) : (
                  <FilePlus2 size={15} className="shrink-0 text-zinc-500" />
                )
              }
              onSubmit={(name) => create('', name, pending.type)}
              onCancel={cancelEdit}
            />
          )}
          {tree.map((node) => (
            <TreeNode key={node.path} node={node} depth={0} ctx={ctx} />
          ))}
          {tree.length === 0 && !pending && (
            <div className="px-3 py-4 text-xs text-zinc-600">
              Empty project — right-click to add a file
            </div>
          )}
        </div>
      )}

      {menu && (
        <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu.node)} onClose={() => setMenu(null)} />
      )}
      {confirming && (
        <ConfirmDelete
          target={confirming}
          onConfirm={() => remove(confirming)}
          onCancel={() => setConfirming(null)}
        />
      )}
    </div>
  );
}
