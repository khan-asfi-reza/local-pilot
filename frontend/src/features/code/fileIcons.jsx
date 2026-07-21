import { Icon, addCollection } from '@iconify/react';
import collection from './vscodeFileIcons.json';

// Register a trimmed slice of the vscode-icons set (the same colorful file
// glyphs VS Code / Material themes use), bundled offline so icons never depend
// on a network fetch. Only the ~30 types we map are included (see the JSON).
addCollection(collection);

// extension -> vscode-icons "file-type-<x>" name.
const BY_EXT = {
  py: 'python', pyw: 'python',
  js: 'js', mjs: 'js', cjs: 'js',
  jsx: 'reactjs', tsx: 'reactts',
  ts: 'typescript', mts: 'typescript', cts: 'typescript',
  go: 'go', rs: 'rust', rb: 'ruby', php: 'php', java: 'java',
  c: 'c', h: 'c',
  cpp: 'cpp', cc: 'cpp', cxx: 'cpp', hpp: 'cpp', hh: 'cpp',
  cs: 'csharp', kt: 'kotlin', kts: 'kotlin', swift: 'swift', lua: 'lua',
  vue: 'vue', svelte: 'svelte',
  html: 'html', htm: 'html',
  css: 'css', scss: 'scss', sass: 'scss', less: 'less',
  md: 'markdown', markdown: 'markdown',
  json: 'json',
  yml: 'yaml', yaml: 'yaml', toml: 'toml',
  sh: 'shell', bash: 'shell', zsh: 'shell',
  sql: 'sql', txt: 'text',
};

// full-filename matches take priority (dotfiles, well-known files).
const BY_NAME = {
  dockerfile: 'docker',
  '.gitignore': 'git',
  '.gitattributes': 'git',
  'package.json': 'npm',
  'package-lock.json': 'npm',
};

// FileIcon renders the VS Code file-type glyph for a filename, or the default
// file icon when the type is unknown.
export function FileIcon({ name, size = 16 }) {
  const lower = (name || '').toLowerCase();
  let key = BY_NAME[lower];
  if (!key && lower.includes('.')) key = BY_EXT[lower.split('.').pop()];
  const icon = key ? `vscode-icons:file-type-${key}` : 'vscode-icons:default-file';
  return <Icon icon={icon} width={size} height={size} className="shrink-0" />;
}

// FolderIcon renders the VS Code folder glyph (open or closed).
export function FolderIcon({ open = false, size = 16 }) {
  return (
    <Icon
      icon={open ? 'vscode-icons:default-folder-opened' : 'vscode-icons:default-folder'}
      width={size}
      height={size}
      className="shrink-0"
    />
  );
}
