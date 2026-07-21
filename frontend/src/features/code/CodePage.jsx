import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { FolderOpen, X, SquareCode } from 'lucide-react';
import { code } from '../../lib/api';
import { FileTree, BrowseModal } from './FileTree';
import { Editor } from './Editor';
import { AgentPanel } from './AgentPanel';

export function CodePage() {
  const navigate = useNavigate();
  const { projectId } = useParams();
  const [root, setRoot] = useState(null);
  const [projectName, setProjectName] = useState(null);
  const [openFiles, setOpenFiles] = useState([]);
  const [activePath, setActivePath] = useState(null);
  const [tree, setTree] = useState(null);
  const [browseOpen, setBrowseOpen] = useState(false);

  const refreshTree = useCallback(async () => {
    if (!root) return;
    try {
      const data = await code.readTree(root);
      setTree(data.tree || []);
    } catch {
      setTree([]);
    }
  }, [root]);

  const openFile = useCallback(
    async (path) => {
      if (!root) return;
      const existing = openFiles.find((f) => f.path === path);
      if (existing) {
        setActivePath(path);
        return;
      }
      try {
        const data = await code.readFile(root, path);
        setOpenFiles((prev) => [...prev, { path, content: data.content, dirty: false }]);
        setActivePath(path);
      } catch (e) {
        console.error('Failed to open file:', e);
      }
    },
    [root, openFiles],
  );

  const changeFile = useCallback((path, content) => {
    setOpenFiles((prev) => prev.map((f) => (f.path === path ? { ...f, content, dirty: true } : f)));
  }, []);

  const saveFile = useCallback(
    async (path) => {
      if (!root) return;
      const file = openFiles.find((f) => f.path === path);
      if (!file) return;
      try {
        await code.writeFile(root, path, file.content);
        setOpenFiles((prev) => prev.map((f) => (f.path === path ? { ...f, dirty: false } : f)));
      } catch (e) {
        console.error('Failed to save file:', e);
      }
    },
    [root, openFiles],
  );

  const closeFile = useCallback(
    (path) => {
      setOpenFiles((prev) => prev.filter((f) => f.path !== path));
      setActivePath((prev) => {
        if (prev !== path) return prev;
        const remaining = openFiles.filter((f) => f.path !== path);
        return remaining.length > 0 ? remaining[remaining.length - 1].path : null;
      });
    },
    [openFiles],
  );

  const reloadOpenFile = useCallback(
    async (path) => {
      if (!root || !path) return;
      let data;
      try {
        data = await code.readFile(root, path);
      } catch {
        return; // file may have been deleted by the agent
      }
      // Refresh the tab from disk, but never clobber unsaved local edits.
      setOpenFiles((prev) =>
        prev.map((f) => (f.path === path && !f.dirty ? { ...f, content: data.content, dirty: false } : f)),
      );
    },
    [root],
  );

  // applyProject loads a project's tree into the editor without touching the URL.
  const applyProject = useCallback(async (proj) => {
    if (!proj) return;
    try {
      setRoot(proj.path);
      setProjectName(proj.name);
      const data = await code.readTree(proj.path);
      setTree(data.tree || []);
    } catch {
      /* backend down */
    }
  }, []);

  // Opening a project also puts its id in the URL, so reload/back restores it.
  const handleProjectOpen = useCallback(
    (proj) => {
      if (!proj) return;
      applyProject(proj);
      if (proj.id) navigate(`/code/${proj.id}`);
    },
    [applyProject, navigate],
  );

  // Restore the project named in the URL (on reload or a shared link).
  useEffect(() => {
    if (!projectId || root) return;
    let cancelled = false;
    code
      .listProjects()
      .then((d) => {
        const proj = (d.projects || []).find((p) => p.id === projectId);
        if (proj && !cancelled) applyProject(proj);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [projectId, root, applyProject]);

  // The empty-state dialog hands back a path; register it as a project, then open.
  const openFromPath = useCallback(
    async (path) => {
      try {
        const proj = await code.openProject(path);
        setBrowseOpen(false);
        handleProjectOpen(proj);
      } catch (e) {
        alert(String(e));
      }
    },
    [handleProjectOpen],
  );

  const handleAgentDone = useCallback(() => {
    refreshTree();
    // Reload every open tab (the agent may have edited more than the active one).
    openFiles.forEach((f) => reloadOpenFile(f.path));
  }, [refreshTree, openFiles, reloadOpenFile]);

  useEffect(() => {
    if (root) refreshTree();
  }, [root, refreshTree]);

  if (!root) {
    return (
      <div className="hero-wash relative flex h-full flex-col items-center justify-center px-6">
        <div className="flex w-full max-w-md flex-col items-center text-center">
          <span className="mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-glow">
            <SquareCode size={30} strokeWidth={2} />
          </span>
          <p className="eyebrow mb-3">Local workspace</p>
          <h1 className="text-3xl font-semibold tracking-tight text-zinc-100">Open a project</h1>
          <p className="mt-2 text-[15px] leading-relaxed text-zinc-400">
            Point the editor at a folder on this machine. Browse the tree, edit files, and let the agent
            work directly in your project.
          </p>
          <button
            type="button"
            onClick={() => setBrowseOpen(true)}
            className="mt-7 inline-flex items-center gap-2 rounded-xl bg-gradient-to-br from-emerald-500 to-teal-600 px-5 py-2.5 text-sm font-medium text-white shadow-glow transition-transform hover:-translate-y-0.5"
          >
            <FolderOpen size={17} strokeWidth={2} />
            Open folder
          </button>
        </div>

        {browseOpen && (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
            onClick={() => setBrowseOpen(false)}
          >
            <div
              className="w-full max-w-md overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-850 shadow-2xl"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
                <span className="text-sm font-medium text-zinc-200">Open a folder</span>
                <button
                  type="button"
                  onClick={() => setBrowseOpen(false)}
                  className="rounded-lg p-1 text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
                  aria-label="Close"
                >
                  <X size={16} />
                </button>
              </div>
              <BrowseModal onOpen={openFromPath} />
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <FileTree
        root={root}
        projectName={projectName}
        tree={tree}
        onOpenFile={openFile}
        onProjectOpen={handleProjectOpen}
        onRefresh={refreshTree}
      />
      <Editor
        openFiles={openFiles}
        activePath={activePath}
        onTabClick={setActivePath}
        onChange={changeFile}
        onSave={saveFile}
        onClose={closeFile}
      />
      <AgentPanel root={root} activePath={activePath} onDone={handleAgentDone} onFileChange={reloadOpenFile} />
    </div>
  );
}
