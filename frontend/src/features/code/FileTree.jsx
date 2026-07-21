import { useCallback, useEffect, useState } from 'react';
import { Folder, FolderOpen, ChevronRight, ChevronDown, FolderUp, FolderSearch } from 'lucide-react';
import { Button } from '../../components/ui/button';
import { code } from '../../lib/api';
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

export function BrowseModal({ onOpen }) {
  const [path, setPath] = useState('');
  const [parent, setParent] = useState(null);
  const [dirs, setDirs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [recent, setRecent] = useState([]);

  const load = useCallback(async (p) => {
    setLoading(true);
    setError(null);
    try {
      const data = await code.browseDir(p);
      setPath(data.path);
      setParent(data.parent);
      setDirs(data.dirs || []);
    } catch (e) {
      setError(String(e));
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    load('');
    code.listProjects().then((d) => setRecent(d.projects || [])).catch(() => {});
  }, [load]);

  return (
    <div className="flex flex-col gap-3 p-4">
      <div className="flex items-center gap-2 text-[13px] text-zinc-400">
        <FolderSearch size={14} className="text-zinc-500" />
        <span className="font-medium text-zinc-300">Browse</span>
      </div>

      <div className="flex items-center gap-1 rounded-lg border border-zinc-800 bg-[#0c0c0e] px-3 py-2">
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
        <span className="truncate text-[13px] text-zinc-200">{path || '~'}</span>
      </div>

      <div className="max-h-60 overflow-y-auto rounded-lg border border-zinc-800 bg-[#0c0c0e]">
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

      <Button variant="default" size="sm" onClick={() => onOpen(path)} disabled={loading}>
        Open this folder
      </Button>

      {recent.length > 0 && (
        <div className="border-t border-zinc-800 pt-3">
          <div className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-600">Recent</div>
          <div className="max-h-36 space-y-0.5 overflow-y-auto">
            {recent.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => onOpen(p.path)}
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-[13px] text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
              >
                <Folder size={14} className="shrink-0 text-zinc-600" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function TreeNode({ node, depth, onOpenFile }) {
  const [expanded, setExpanded] = useState(depth === 0);

  if (node.type === 'dir') {
    return (
      <div>
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="flex w-full items-center gap-1 rounded-lg px-2 py-1 text-left text-[13px] text-zinc-300 hover:bg-zinc-800/40"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
        >
          {expanded ? <ChevronDown size={14} className="shrink-0 text-zinc-500" /> : <ChevronRight size={14} className="shrink-0 text-zinc-500" />}
          {expanded ? <FolderOpen size={15} className="shrink-0 text-amber-400/70" /> : <Folder size={15} className="shrink-0 text-amber-400/70" />}
          <span className="truncate">{node.name}</span>
        </button>
        {expanded && node.children?.map((child) => (
          <TreeNode key={child.path} node={child} depth={depth + 1} onOpenFile={onOpenFile} />
        ))}
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={() => onOpenFile(node.path)}
      className="flex w-full items-center gap-1 rounded-lg px-2 py-1 text-left text-[13px] text-zinc-400 hover:bg-zinc-800/40 hover:text-zinc-200"
      style={{ paddingLeft: `${depth * 16 + 24}px` }}
    >
      <FileIcon name={node.name} size={15} />
      <span className="truncate">{node.name}</span>
    </button>
  );
}

export function FileTree({ root, projectName, tree, onOpenFile, onProjectOpen, onRefresh }) {
  const [browseOpen, setBrowseOpen] = useState(false);

  const handleOpen = useCallback(async (path) => {
    try {
      const proj = await code.openProject(path);
      setBrowseOpen(false);
      onProjectOpen?.(proj);
    } catch (e) {
      alert(String(e));
    }
  }, [onProjectOpen]);

  return (
    <div className="flex h-full w-[260px] shrink-0 flex-col border-r border-zinc-800 bg-[#101012]">
      <div className="flex items-center justify-between px-3 py-3">
        <span className="text-sm font-medium text-zinc-300">{projectName || 'Files'}</span>
        <button
          type="button"
          onClick={() => setBrowseOpen((v) => !v)}
          className="rounded-lg px-2 py-1 text-[13px] text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
        >
          {browseOpen ? 'Close' : 'Open folder'}
        </button>
      </div>

      {browseOpen && (
        <div className="border-b border-zinc-800">
          <BrowseModal onOpen={handleOpen} />
        </div>
      )}

      {root && tree && (
        <div className="flex-1 overflow-y-auto px-1 py-1">
          {tree.map((node) => (
            <TreeNode key={node.path} node={node} depth={0} onOpenFile={onOpenFile} />
          ))}
          {tree.length === 0 && (
            <div className="px-3 py-4 text-xs text-zinc-600">Empty project</div>
          )}
        </div>
      )}

      {!root && !browseOpen && (
        <div className="flex flex-1 items-center justify-center px-4">
          <div className="text-center">
            <p className="text-sm text-zinc-500">Open a folder to get started</p>
          </div>
        </div>
      )}
    </div>
  );
}
