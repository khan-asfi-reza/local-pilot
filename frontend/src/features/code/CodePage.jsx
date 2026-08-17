import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { code } from '../../lib/api';
import { FileTree } from './FileTree';
import { Editor } from './Editor';
import { AgentThreads } from './AgentThreads';
import { DiffReview } from './DiffReview';
import { StartScreen } from './StartScreen';
import { TerminalPanel } from './TerminalPanel';

const TERM_HEIGHT_KEY = 'code:terminalHeight';
const TREE_POLL_MS = 2500;

export function CodePage() {
  const navigate = useNavigate();
  const { projectId } = useParams();
  const [root, setRoot] = useState(null);
  const [projectName, setProjectName] = useState(null);
  const [openFiles, setOpenFiles] = useState([]);
  const [activePath, setActivePath] = useState(null);
  const [tree, setTree] = useState(null);
  // A freshly created project can carry a first instruction for the agent.
  const [initialPrompt, setInitialPrompt] = useState(null);
  // The last path the agent touched, so the tree can expand down to it.
  const [revealPath, setRevealPath] = useState(null);
  const [terminalOpen, setTerminalOpen] = useState(false);
  const [terminalHeight, setTerminalHeight] = useState(
    () => Number(localStorage.getItem(TERM_HEIGHT_KEY)) || 240,
  );
  // Ask-mode review: the pending confirm (bubbled up from AgentPanel) and whether
  // its diff is showing in the center pane instead of the editor.
  const [confirmState, setConfirmState] = useState(null); // { confirm, respond } | null
  const [reviewOpen, setReviewOpen] = useState(false);
  const treeJsonRef = useRef('');

  useEffect(() => {
    localStorage.setItem(TERM_HEIGHT_KEY, String(terminalHeight));
  }, [terminalHeight]);

  // refreshTree re-reads the tree but only re-renders when it actually changed,
  // so the 2.5s poll below stays invisible.
  const refreshTree = useCallback(async () => {
    if (!root) return;
    try {
      const data = await code.readTree(root);
      const next = JSON.stringify(data.tree || []);
      if (next !== treeJsonRef.current) {
        treeJsonRef.current = next;
        setTree(data.tree || []);
      }
    } catch {
      /* backend down; keep the last good tree */
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

  const closeFile = useCallback((path) => {
    setOpenFiles((prev) => {
      const remaining = prev.filter((f) => f.path !== path);
      setActivePath((cur) => (cur === path ? remaining[remaining.length - 1]?.path ?? null : cur));
      return remaining;
    });
  }, []);

  // A file deleted from the tree cannot stay open in a tab.
  const handleDeleted = useCallback(
    (path) => {
      setOpenFiles((prev) => {
        const remaining = prev.filter((f) => f.path !== path && !f.path.startsWith(`${path}/`));
        setActivePath((cur) =>
          cur === path || cur?.startsWith(`${path}/`) ? remaining[remaining.length - 1]?.path ?? null : cur,
        );
        return remaining;
      });
    },
    [],
  );

  // A rename has to follow through to the open tabs, including files inside a
  // renamed folder.
  const handleRenamed = useCallback((from, to) => {
    const moved = (p) => (p === from ? to : p.startsWith(`${from}/`) ? to + p.slice(from.length) : p);
    setOpenFiles((prev) => prev.map((f) => ({ ...f, path: moved(f.path) })));
    setActivePath((cur) => (cur ? moved(cur) : cur));
  }, []);

  const reloadOpenFile = useCallback(
    async (path) => {
      if (!root || !path) return;
      setRevealPath(path);
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
    treeJsonRef.current = '';
    setOpenFiles([]);
    setActivePath(null);
    setTree(null);
    setRoot(proj.path);
    setProjectName(proj.name);
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

  // The start screen hands back a folder path; register it as a project, then
  // open it. Errors bubble so the picker can show them.
  const openFromPath = useCallback(
    async (path) => {
      const proj = await code.openProject(path);
      handleProjectOpen(proj);
    },
    [handleProjectOpen],
  );

  // A new project opens straight away; its first instruction (if any) is a
  // { text, attachments } payload handed to the agent panel, which sends it once
  // the project is loaded — the spec rides along as an attachment, not a file.
  const handleProjectCreated = useCallback(
    (proj, payload) => {
      setInitialPrompt(payload || null);
      handleProjectOpen(proj);
    },
    [handleProjectOpen],
  );

  // Closing a project drops back to the start screen — the one place that opens
  // or creates projects.
  const closeProject = useCallback(() => {
    setRoot(null);
    setProjectName(null);
    setTree(null);
    setOpenFiles([]);
    setActivePath(null);
    setTerminalOpen(false);
    treeJsonRef.current = '';
    navigate('/code');
  }, [navigate]);

  const handleAgentDone = useCallback(() => {
    refreshTree();
    // Reload every open tab (the agent may have edited more than the active one).
    openFiles.forEach((f) => reloadOpenFile(f.path));
    setReviewOpen(false); // the run finished; drop back to the editor
  }, [refreshTree, openFiles, reloadOpenFile]);

  useEffect(() => {
    if (root) refreshTree();
  }, [root, refreshTree]);

  // Poll while a project is open, so files created by the agent or by a command
  // in the terminal show up on their own.
  useEffect(() => {
    if (!root) return undefined;
    const timer = setInterval(() => {
      if (!document.hidden) refreshTree();
    }, TREE_POLL_MS);
    return () => clearInterval(timer);
  }, [root, refreshTree]);

  useEffect(() => {
    if (!root) return undefined;
    const onKey = (e) => {
      if (e.key === '`' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        setTerminalOpen((v) => !v);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [root]);

  if (!root) {
    return (
      <StartScreen
        onOpenProject={handleProjectOpen}
        onOpenPath={openFromPath}
        onCreated={handleProjectCreated}
      />
    );
  }

  return (
    <div className="flex h-full overflow-hidden">
      <FileTree
        root={root}
        projectName={projectName}
        tree={tree}
        activePath={activePath}
        revealPath={revealPath}
        onOpenFile={openFile}
        onCloseProject={closeProject}
        onRefresh={refreshTree}
        onDeleted={handleDeleted}
        onRenamed={handleRenamed}
      />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        {reviewOpen && confirmState ? (
          <DiffReview
            confirm={confirmState.confirm}
            onApprove={(note) => confirmState.respond('approve', note)}
            onReject={(note) => confirmState.respond('decline', note)}
            onClose={() => setReviewOpen(false)}
          />
        ) : (
          <Editor
            openFiles={openFiles}
            activePath={activePath}
            onTabClick={setActivePath}
            onChange={changeFile}
            onSave={saveFile}
            onClose={closeFile}
            onToggleTerminal={() => setTerminalOpen((v) => !v)}
          />
        )}
        {terminalOpen && (
          <TerminalPanel
            root={root}
            height={terminalHeight}
            onHeightChange={setTerminalHeight}
            onClose={() => setTerminalOpen(false)}
          />
        )}
      </div>
      <AgentThreads
        root={root}
        activePath={activePath}
        tree={tree}
        initialPrompt={initialPrompt}
        onInitialPromptSent={() => setInitialPrompt(null)}
        onDone={handleAgentDone}
        onFileChange={reloadOpenFile}
        onActivity={refreshTree}
        onConfirmChange={setConfirmState}
        onViewDiff={() => setReviewOpen(true)}
      />
    </div>
  );
}
