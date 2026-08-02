import { StreamLanguage } from '@codemirror/language';

// Syntax support for the editor and the diff viewer.
//
// Every loader below names its package with a literal specifier, so the bundler
// resolves and code-splits it at build time. @codemirror/language-data was used
// before, but its loaders are dynamic imports the bundler cannot resolve, so
// anything outside the lang packages listed in package.json silently fell back to
// plain text — that is why C files had no colours.

const cpp = () => import('@codemirror/lang-cpp').then((m) => m.cpp());
const js = (config) => () => import('@codemirror/lang-javascript').then((m) => m.javascript(config));
const python = () => import('@codemirror/lang-python').then((m) => m.python());
const go = () => import('@codemirror/lang-go').then((m) => m.go());
const rust = () => import('@codemirror/lang-rust').then((m) => m.rust());
const java = () => import('@codemirror/lang-java').then((m) => m.java());
const php = () => import('@codemirror/lang-php').then((m) => m.php());
const html = () => import('@codemirror/lang-html').then((m) => m.html());
const vue = () => import('@codemirror/lang-vue').then((m) => m.vue());
const css = () => import('@codemirror/lang-css').then((m) => m.css());
const less = () => import('@codemirror/lang-less').then((m) => m.less());
const sass = (indented) => () => import('@codemirror/lang-sass').then((m) => m.sass({ indented }));
const json = () => import('@codemirror/lang-json').then((m) => m.json());
const yaml = () => import('@codemirror/lang-yaml').then((m) => m.yaml());
const xml = () => import('@codemirror/lang-xml').then((m) => m.xml());
const markdown = () => import('@codemirror/lang-markdown').then((m) => m.markdown());
const sql = () => import('@codemirror/lang-sql').then((m) => m.sql());
const wast = () => import('@codemirror/lang-wast').then((m) => m.wast());

// CodeMirror 5 stream parsers, wrapped as languages. Bare specifiers cannot be
// built from a variable, so each mode gets its own literal import.
const stream = (load, pick) => () => load().then((m) => StreamLanguage.define(pick(m)));

const clike = (name) => stream(() => import('@codemirror/legacy-modes/mode/clike'), (m) => m[name]);
const shell = stream(() => import('@codemirror/legacy-modes/mode/shell'), (m) => m.shell);
const properties = stream(() => import('@codemirror/legacy-modes/mode/properties'), (m) => m.properties);
const toml = stream(() => import('@codemirror/legacy-modes/mode/toml'), (m) => m.toml);
const ruby = stream(() => import('@codemirror/legacy-modes/mode/ruby'), (m) => m.ruby);
const perl = stream(() => import('@codemirror/legacy-modes/mode/perl'), (m) => m.perl);
const lua = stream(() => import('@codemirror/legacy-modes/mode/lua'), (m) => m.lua);
const swift = stream(() => import('@codemirror/legacy-modes/mode/swift'), (m) => m.swift);
const rlang = stream(() => import('@codemirror/legacy-modes/mode/r'), (m) => m.r);
const powershell = stream(() => import('@codemirror/legacy-modes/mode/powershell'), (m) => m.powerShell);
const dockerfile = stream(() => import('@codemirror/legacy-modes/mode/dockerfile'), (m) => m.dockerFile);
const cmake = stream(() => import('@codemirror/legacy-modes/mode/cmake'), (m) => m.cmake);
const protobuf = stream(() => import('@codemirror/legacy-modes/mode/protobuf'), (m) => m.protobuf);
const haskell = stream(() => import('@codemirror/legacy-modes/mode/haskell'), (m) => m.haskell);
const erlang = stream(() => import('@codemirror/legacy-modes/mode/erlang'), (m) => m.erlang);
const clojure = stream(() => import('@codemirror/legacy-modes/mode/clojure'), (m) => m.clojure);
const julia = stream(() => import('@codemirror/legacy-modes/mode/julia'), (m) => m.julia);
const groovy = stream(() => import('@codemirror/legacy-modes/mode/groovy'), (m) => m.groovy);
const verilog = stream(() => import('@codemirror/legacy-modes/mode/verilog'), (m) => m.verilog);
const stex = stream(() => import('@codemirror/legacy-modes/mode/stex'), (m) => m.stex);
const nginx = stream(() => import('@codemirror/legacy-modes/mode/nginx'), (m) => m.nginx);
const diffMode = stream(() => import('@codemirror/legacy-modes/mode/diff'), (m) => m.diff);

// One loader per file extension. Keys are lowercase, without the dot.
const BY_EXT = {
  c: cpp,
  h: cpp,
  ino: cpp,
  cpp,
  cc: cpp,
  cxx: cpp,
  'c++': cpp,
  hpp: cpp,
  hh: cpp,
  hxx: cpp,
  'h++': cpp,
  m: clike('objectiveC'),
  mm: clike('objectiveCpp'),
  js: js(),
  mjs: js(),
  cjs: js(),
  jsx: js({ jsx: true }),
  ts: js({ typescript: true }),
  mts: js({ typescript: true }),
  cts: js({ typescript: true }),
  tsx: js({ jsx: true, typescript: true }),
  py: python,
  pyw: python,
  pyi: python,
  go,
  rs: rust,
  java,
  kt: clike('kotlin'),
  kts: clike('kotlin'),
  cs: clike('csharp'),
  scala: clike('scala'),
  dart: clike('dart'),
  php,
  phtml: php,
  html,
  htm: html,
  vue,
  css,
  less,
  scss: sass(false),
  sass: sass(true),
  json,
  jsonc: json,
  json5: json,
  yaml,
  yml: yaml,
  xml,
  svg: xml,
  xsd: xml,
  plist: xml,
  md: markdown,
  markdown,
  mdx: markdown,
  sql,
  wat: wast,
  wast,
  sh: shell,
  bash: shell,
  zsh: shell,
  fish: shell,
  ps1: powershell,
  toml,
  ini: properties,
  cfg: properties,
  conf: properties,
  env: properties,
  properties,
  lua,
  rb: ruby,
  rake: ruby,
  gemspec: ruby,
  pl: perl,
  pm: perl,
  swift,
  r: rlang,
  proto: protobuf,
  hs: haskell,
  erl: erlang,
  clj: clojure,
  cljs: clojure,
  jl: julia,
  groovy,
  gradle: groovy,
  v: verilog,
  sv: verilog,
  tex: stex,
  diff: diffMode,
  patch: diffMode,
};

// Files whose whole name decides the language (no useful extension).
const BY_NAME = {
  dockerfile,
  containerfile: dockerfile,
  'cmakelists.txt': cmake,
  gemfile: ruby,
  rakefile: ruby,
  'nginx.conf': nginx,
  '.bashrc': shell,
  '.zshrc': shell,
  '.bash_profile': shell,
  '.profile': shell,
  '.env': properties,
  '.gitconfig': properties,
};

const cache = new Map();

function basename(path) {
  return String(path || '').split('/').pop() || '';
}

// loaderFor picks the loader for a filename, or null when the type is unknown.
function loaderFor(path) {
  const name = basename(path).toLowerCase();
  if (BY_NAME[name]) return BY_NAME[name];
  if (name.startsWith('dockerfile')) return dockerfile;
  if (name.startsWith('.env.')) return properties;
  const dot = name.lastIndexOf('.');
  if (dot <= 0) return null;
  return BY_EXT[name.slice(dot + 1)] || null;
}

// loadLanguage resolves a filename to a CodeMirror language extension (a
// LanguageSupport or a StreamLanguage), or null for plain text. Each parser is a
// separate lazily-loaded chunk, cached after the first file of that type.
export async function loadLanguage(path) {
  const loader = loaderFor(path);
  if (!loader) return null;
  if (cache.has(loader)) return cache.get(loader);
  const pending = loader().catch(() => null);
  cache.set(loader, pending);
  const support = await pending;
  cache.set(loader, support);
  return support;
}

// languageOf unwraps whatever loadLanguage returned into the Language whose
// parser the diff viewer uses for highlighting.
export function languageOf(support) {
  if (!support) return null;
  return support.language ?? (support.parser ? support : null);
}
