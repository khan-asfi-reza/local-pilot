import { useCallback, useEffect, useState } from 'react';
import { Compass } from 'lucide-react';
import { code } from '../../lib/api';
import { FileTree } from './FileTree';
import { Editor } from './Editor';
import { AgentPanel } from './AgentPanel';

export function CodePage() {
  const [root, setRoot] = useState(null);
  const [projectName, setProjectName] = useState(null);
  const [openFiles, setOpenFiles] = useState([]);
  const [activePath, setActivePath] = useState(null);
  const [tree, setTree] = useState(null);

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
      try {
        const data = await code.readFile(root, path);
        setOpenFiles((prev) => prev.map((f) => (f.path === path ? { ...f, content: data.content, dirty: false } : f)));
      } catch {
        /* file may have been deleted by agent */
      }
    },
    [root],
  );

  const handleProjectOpen = useCallback(async (proj) => {
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

  const handleAgentDone = useCallback(() => {
    refreshTree();
    if (activePath) reloadOpenFile(activePath);
  }, [refreshTree, activePath, reloadOpenFile]);

  useEffect(() => {
    if (root) refreshTree();
  }, [root, refreshTree]);

  return (
    <div className="flex h-full">
      {!root ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-6">
          <div className="flex flex-col items-center gap-3">
            <span className="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-500 to-teal-600 text-white shadow-lg">
              <Compass size={32} strokeWidth={2.2} />
            </span>
            <h1 className="text-2xl font-bold tracking-tight text-zinc-100">Code</h1>
            <p className="text-sm text-zinc-500">Open a folder to start editing</p>
          </div>
          <FileTree
            root={root}
            projectName={projectName}
            tree={tree}
            onOpenFile={openFile}
            onProjectOpen={handleProjectOpen}
            onRefresh={refreshTree}
          />
        </div>
      ) : (
        <>
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
          <AgentPanel root={root} activePath={activePath} onDone={handleAgentDone} />
        </>
      )}
    </div>
  );
}
