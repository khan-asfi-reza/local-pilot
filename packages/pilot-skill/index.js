#!/usr/bin/env node
// pilot-skill — install local-pilot skills into the user's local skills directory.
//
// A "skill" is a folder containing a SKILL.md file (YAML frontmatter with `name`
// and `description`, followed by instructions). Shipped skills live in the
// managed default directory and are refreshed on upgrade; skills installed here
// live in a separate "skills_local" directory that upgrades never touch, and the
// harness scans both.
//
// Usage:
//   npx pilot-skill add <source>      Install a skill.
//   npx pilot-skill list              List installed local skills.
//   npx pilot-skill remove <name>     Remove an installed skill.
//
// <source> may be:
//   owner/repo                 GitHub repo whose root is the skill (has SKILL.md)
//   owner/repo/path/to/skill   GitHub repo with the skill in a subfolder
//   owner/repo#branch          pin a branch/tag (works with a subfolder too)
//   https://…(.git) / git@…    any git remote (optionally with #branch)
//   ./path or /abs/path        a local folder to copy
//
// No dependencies; uses `git` for remote sources.

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

// localSkillsDir mirrors the Go appdir layout so both agree on where skills live.
function localSkillsDir() {
  const platform = process.platform;
  if (platform === 'win32') {
    const base = process.env.LOCALAPPDATA || process.env.APPDATA;
    if (base) return path.join(base, 'localpilot', 'skills_local');
  } else if (platform === 'darwin') {
    return path.join(os.homedir(), '.localpilot', 'skills_local');
  } else {
    const base = process.env.XDG_DATA_HOME;
    if (base) return path.join(base, 'localpilot', 'skills_local');
    return path.join(os.homedir(), '.local', 'share', 'localpilot', 'skills_local');
  }
  return path.join(os.homedir(), '.localpilot', 'skills_local');
}

function fail(msg) {
  console.error(`\x1b[31merror\x1b[0m ${msg}`);
  process.exit(1);
}

// safeSkillDest validates a skill name (from frontmatter or the CLI) and returns
// its destination inside the skills root. It refuses anything that is not a
// simple identifier and confirms the resolved path stays inside the root, so a
// crafted name cannot traverse out and copy or delete arbitrary files.
function safeSkillDest(name) {
  if (typeof name !== 'string' || !/^[A-Za-z0-9_.-]+$/.test(name) || name === '.' || name === '..') {
    fail(`invalid skill name "${name}": use only letters, digits, dash, underscore, and dot`);
  }
  const root = path.resolve(localSkillsDir());
  const dest = path.resolve(root, name);
  if (dest !== path.join(root, name) || !dest.startsWith(root + path.sep)) {
    fail(`skill name "${name}" would escape the skills directory`);
  }
  return dest;
}

function info(msg) {
  console.log(msg);
}

// parseFrontmatter pulls name and description out of a SKILL.md frontmatter block.
function parseFrontmatter(text) {
  const lines = text.split('\n');
  if (lines[0]?.trim() !== '---') return {};
  const out = {};
  for (const ln of lines.slice(1)) {
    if (ln.trim() === '---') break;
    const m = ln.match(/^\s*(name|description)\s*:\s*(.*)$/);
    if (m) out[m[1]] = m[2].trim();
  }
  return out;
}

function copyDir(src, dst) {
  fs.mkdirSync(dst, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    if (entry.name === '.git') continue;
    const s = path.join(src, entry.name);
    const d = path.join(dst, entry.name);
    if (entry.isDirectory()) copyDir(s, d);
    else if (entry.isFile()) fs.copyFileSync(s, d);
  }
}

// resolveSource turns a source string into { repoUrl|localPath, subdir, ref }.
function resolveSource(source) {
  // Local path: exists on disk, or looks like a path.
  if (source.startsWith('.') || source.startsWith('/') || source.startsWith('~')) {
    const local = source.startsWith('~') ? path.join(os.homedir(), source.slice(1)) : source;
    return { localPath: path.resolve(local) };
  }
  // Split an optional #ref off the end.
  let ref = '';
  const hash = source.indexOf('#');
  if (hash !== -1) {
    ref = source.slice(hash + 1);
    source = source.slice(0, hash);
  }
  // Full git URL.
  if (source.includes('://') || source.startsWith('git@')) {
    return { repoUrl: source, subdir: '', ref };
  }
  // GitHub shorthand: owner/repo[/sub/path].
  const parts = source.split('/').filter(Boolean);
  if (parts.length < 2) fail(`could not understand source "${source}" (expected owner/repo, a git URL, or a path)`);
  const [owner, repo, ...rest] = parts;
  return { repoUrl: `https://github.com/${owner}/${repo}.git`, subdir: rest.join('/'), ref };
}

function haveGit() {
  try {
    execFileSync('git', ['--version'], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// materialize returns a local folder path for the skill source, cloning if remote.
function materialize(resolved) {
  if (resolved.localPath) {
    if (!fs.existsSync(resolved.localPath)) fail(`path not found: ${resolved.localPath}`);
    return { dir: resolved.localPath, cleanup: null };
  }
  if (!haveGit()) fail('git is required to install from a remote source. Install git or use a local path.');
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'pilot-skill-'));
  const args = ['clone', '--depth', '1'];
  if (resolved.ref) args.push('--branch', resolved.ref);
  args.push(resolved.repoUrl, tmp);
  info(`Cloning ${resolved.repoUrl}${resolved.ref ? ` @ ${resolved.ref}` : ''} …`);
  try {
    execFileSync('git', args, { stdio: ['ignore', 'ignore', 'pipe'] });
  } catch (e) {
    try { fs.rmSync(tmp, { recursive: true, force: true }); } catch {}
    fail(`git clone failed: ${e.stderr ? e.stderr.toString().trim() : e.message}`);
  }
  const dir = resolved.subdir ? path.join(tmp, resolved.subdir) : tmp;
  return { dir, cleanup: () => { try { fs.rmSync(tmp, { recursive: true, force: true }); } catch {} } };
}

function add(source) {
  if (!source) fail('usage: pilot-skill add <owner/repo[/path]|git-url|path>');
  const resolved = resolveSource(source);
  const { dir, cleanup } = materialize(resolved);
  try {
    const skillFile = path.join(dir, 'SKILL.md');
    if (!fs.existsSync(skillFile)) {
      fail(`no SKILL.md found in ${resolved.subdir || 'the source root'}. Point the source at the folder that contains SKILL.md.`);
    }
    const meta = parseFrontmatter(fs.readFileSync(skillFile, 'utf8'));
    const name = meta.name || path.basename(dir);
    const dest = safeSkillDest(name);
    if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
    copyDir(dir, dest);
    info(`\x1b[32m✓\x1b[0m Installed skill "\x1b[1m${name}\x1b[0m"${meta.description ? ` — ${meta.description}` : ''}`);
    info(`  → ${dest}`);
    info('  Restart the app (or harness) so the skill is picked up.');
  } finally {
    cleanup?.();
  }
}

function list() {
  const dir = localSkillsDir();
  if (!fs.existsSync(dir)) return info('No local skills installed yet.');
  const names = fs.readdirSync(dir, { withFileTypes: true }).filter((e) => e.isDirectory());
  if (names.length === 0) return info('No local skills installed yet.');
  info(`Local skills (${dir}):`);
  for (const e of names) {
    const f = path.join(dir, e.name, 'SKILL.md');
    let desc = '';
    if (fs.existsSync(f)) desc = parseFrontmatter(fs.readFileSync(f, 'utf8')).description || '';
    info(`  \x1b[1m${e.name}\x1b[0m${desc ? ` — ${desc}` : ''}`);
  }
}

function remove(name) {
  if (!name) fail('usage: pilot-skill remove <name>');
  const dest = safeSkillDest(name);
  if (!fs.existsSync(dest)) fail(`skill "${name}" is not installed`);
  fs.rmSync(dest, { recursive: true, force: true });
  info(`\x1b[32m✓\x1b[0m Removed skill "${name}"`);
}

function help() {
  info(`pilot-skill — manage local-pilot skills

Usage:
  npx pilot-skill add <source>     Install a skill (owner/repo[/path], git URL, or local path)
  npx pilot-skill list             List installed local skills
  npx pilot-skill remove <name>    Remove an installed skill

Examples:
  npx pilot-skill add acme/cool-skill
  npx pilot-skill add acme/skills/pdf-writer#main
  npx pilot-skill add https://github.com/acme/skills.git#v1
  npx pilot-skill add ./my-skill

Skills install to: ${localSkillsDir()}`);
}

const [cmd, arg] = process.argv.slice(2);
switch (cmd) {
  case 'add': add(arg); break;
  case 'list': case 'ls': list(); break;
  case 'remove': case 'rm': remove(arg); break;
  case 'help': case '--help': case '-h': case undefined: help(); break;
  default: fail(`unknown command "${cmd}". Run "pilot-skill help".`);
}
