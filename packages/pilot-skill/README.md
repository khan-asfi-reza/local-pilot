# pilot-skill

Install [local-pilot](../../) skills from GitHub, git, or a local path — the same
`npx … add` flow other tools use.

## Skills: default vs local

- **Default skills** ship with local-pilot (the repo's `skills/`, seeded into the
  managed data directory). They are refreshed on upgrade, so don't edit them by
  hand — your changes would be overwritten.
- **Local skills** are the ones you install. They live in a separate
  `skills_local` directory that upgrades never touch.

The harness scans **both** and offers them all through `load_skill`. A local skill
with the same `name` as a default one shadows it.

Data directory (per OS):

| OS      | Local skills path                                    |
| ------- | ---------------------------------------------------- |
| macOS   | `~/.localpilot/skills_local`                         |
| Linux   | `${XDG_DATA_HOME:-~/.local/share}/localpilot/skills_local` |
| Windows | `%LOCALAPPDATA%\localpilot\skills_local`             |

## Usage

```bash
npx pilot-skill add <source>     # install a skill
npx pilot-skill list             # list installed local skills
npx pilot-skill remove <name>    # remove one
```

`<source>` can be:

| Source                          | Meaning                                        |
| ------------------------------- | ---------------------------------------------- |
| `owner/repo`                    | GitHub repo whose root has `SKILL.md`          |
| `owner/repo/path/to/skill`      | skill in a subfolder of a GitHub repo          |
| `owner/repo#branch`             | pin a branch or tag (works with a subfolder)   |
| `https://…​.git`, `git@…`       | any git remote (optionally `#branch`)          |
| `./path`, `/abs/path`           | a local folder to copy                         |

Examples:

```bash
npx pilot-skill add acme/cool-skill
npx pilot-skill add acme/skills/pdf-writer#main
npx pilot-skill add ./my-skill
```

Remote sources use `git` (clone `--depth 1`); a local path is copied directly.
Restart the app (or the harness) after installing so the new skill is scanned.

## What a skill is

A folder with a `SKILL.md`: YAML frontmatter (`name`, `description`) then the
instructions the model loads on demand.

```markdown
---
name: pdf-writer
description: Fill and generate PDF forms from a template.
---

# PDF Writer

...instructions...
```
