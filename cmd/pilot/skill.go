package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"harness/harness/appdir"
)

var (
	skillNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	fmKeyRe     = regexp.MustCompile(`^\s*(name|description)\s*:\s*(.*)$`)
)

type skillSource struct {
	localPath string
	repoURL   string
	subdir    string
	ref       string
}

// skillCmd handles `pilot skill add|list|remove`.
func skillCmd(args []string) error {
	if len(args) == 0 {
		fmt.Print(skillHelp())
		return nil
	}
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	switch args[0] {
	case "add":
		return skillAdd(arg)
	case "list", "ls":
		return skillList()
	case "remove", "rm":
		return skillRemove(arg)
	case "help", "-h", "--help":
		fmt.Print(skillHelp())
		return nil
	default:
		return fmt.Errorf("unknown: pilot skill %s (try add, list, or remove)", args[0])
	}
}

// localSkillsDir is the user-owned skills directory the harness scans alongside
// the shipped defaults; upgrades never touch it.
func localSkillsDir() string { return filepath.Join(appdir.Dir(), "skills_local") }

// safeSkillDest validates a skill name and returns its destination inside the
// local skills root, refusing anything that could traverse out of it.
func safeSkillDest(name string) (string, error) {
	if name == "" || name == "." || name == ".." || !skillNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid skill name %q: use only letters, digits, dash, underscore, and dot", name)
	}
	root := localSkillsDir()
	dest := filepath.Join(root, name)
	if filepath.Dir(dest) != filepath.Clean(root) {
		return "", fmt.Errorf("skill name %q would escape the skills directory", name)
	}
	return dest, nil
}

// parseFrontmatter pulls name and description out of a SKILL.md frontmatter block.
func parseFrontmatter(text string) (name, desc string) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			break
		}
		if m := fmKeyRe.FindStringSubmatch(ln); m != nil {
			switch m[1] {
			case "name":
				name = strings.TrimSpace(m[2])
			case "description":
				desc = strings.TrimSpace(m[2])
			}
		}
	}
	return name, desc
}

// resolveSource turns a source string into a git remote (with optional subdir
// and ref) or a local path.
func resolveSource(source string) (skillSource, error) {
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") {
		local := source
		if strings.HasPrefix(source, "~") {
			if h, err := os.UserHomeDir(); err == nil {
				local = filepath.Join(h, source[1:])
			}
		}
		abs, err := filepath.Abs(local)
		if err != nil {
			return skillSource{}, err
		}
		return skillSource{localPath: abs}, nil
	}
	ref := ""
	if i := strings.Index(source, "#"); i != -1 {
		ref = source[i+1:]
		source = source[:i]
	}
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
		return skillSource{repoURL: source, ref: ref}, nil
	}
	var parts []string
	for _, p := range strings.Split(source, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return skillSource{}, fmt.Errorf("could not understand source %q (expected owner/repo, a git URL, or a path)", source)
	}
	return skillSource{
		repoURL: fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]),
		subdir:  strings.Join(parts[2:], "/"),
		ref:     ref,
	}, nil
}

// materialize returns a local folder for the skill source, cloning to a temp dir
// when it is remote; the returned cleanup removes any temp dir.
func materialize(src skillSource) (dir string, cleanup func(), err error) {
	if src.localPath != "" {
		if _, e := os.Stat(src.localPath); e != nil {
			return "", func() {}, fmt.Errorf("path not found: %s", src.localPath)
		}
		return src.localPath, func() {}, nil
	}
	if _, e := exec.LookPath("git"); e != nil {
		return "", func() {}, fmt.Errorf("git is required to install from a remote source. Install git or use a local path")
	}
	tmp, e := os.MkdirTemp("", "pilot-skill-")
	if e != nil {
		return "", func() {}, e
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	args := []string{"clone", "--depth", "1"}
	if src.ref != "" {
		args = append(args, "--branch", src.ref)
	}
	args = append(args, src.repoURL, tmp)
	label := src.repoURL
	if src.ref != "" {
		label += " @ " + src.ref
	}
	fmt.Println(dim("… cloning " + label))
	cmd := exec.Command("git", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if e := cmd.Run(); e != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git clone failed: %s", strings.TrimSpace(stderr.String()))
	}
	dir = tmp
	if src.subdir != "" {
		dir = filepath.Join(tmp, filepath.FromSlash(src.subdir))
		if rel, e := filepath.Rel(tmp, dir); e != nil || strings.HasPrefix(rel, "..") {
			cleanup()
			return "", func() {}, fmt.Errorf("subfolder %q escapes the repository", src.subdir)
		}
	}
	return dir, cleanup, nil
}

// skillAdd installs a skill from owner/repo[/path], a git URL, or a local path.
func skillAdd(source string) error {
	if source == "" {
		return fmt.Errorf("usage: pilot skill add <owner/repo[/path]|git-url|path>")
	}
	src, err := resolveSource(source)
	if err != nil {
		return err
	}
	dir, cleanup, err := materialize(src)
	if err != nil {
		return err
	}
	defer cleanup()

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		where := "the source root"
		if src.subdir != "" {
			where = src.subdir
		}
		return fmt.Errorf("no SKILL.md found in %s. Point the source at the folder that contains SKILL.md", where)
	}
	name, desc := parseFrontmatter(string(data))
	if name == "" {
		name = filepath.Base(dir)
	}
	dest, err := safeSkillDest(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := copyDir(dir, dest); err != nil {
		return err
	}
	line := green("✓ ") + "installed skill " + bold(name)
	if desc != "" {
		line += " — " + desc
	}
	fmt.Println(line)
	fmt.Println(dim("  → " + dest))
	fmt.Println(dim("  restart the app (or harness) so the skill is picked up"))
	return nil
}

// skillList prints the installed local skills.
func skillList() error {
	dir := localSkillsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println(dim("no local skills installed yet"))
		return nil
	}
	var names []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e)
		}
	}
	if len(names) == 0 {
		fmt.Println(dim("no local skills installed yet"))
		return nil
	}
	fmt.Println(bold("Local skills") + dim("  ("+dir+")"))
	for _, e := range names {
		desc := ""
		if b, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil {
			_, desc = parseFrontmatter(string(b))
		}
		line := "  " + cyan(e.Name())
		if desc != "" {
			line += dim("  "+desc)
		}
		fmt.Println(line)
	}
	return nil
}

// skillRemove deletes an installed local skill by name.
func skillRemove(name string) error {
	if name == "" {
		return fmt.Errorf("usage: pilot skill remove <name>")
	}
	dest, err := safeSkillDest(name)
	if err != nil {
		return err
	}
	if _, e := os.Stat(dest); e != nil {
		return fmt.Errorf("skill %q is not installed", name)
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	fmt.Println(green("✓ ") + "removed skill " + bold(name))
	return nil
}

// copyDir recursively copies src into dst, skipping any .git directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else if e.Type().IsRegular() {
			if err := copyFile(s, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a single regular file.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// skillHelp is the colored help for `pilot skill`.
func skillHelp() string {
	var b strings.Builder
	b.WriteString("\n" + bold("pilot skill") + dim("  ·  install local skills (no npm, no network unless remote)") + "\n\n")
	b.WriteString(bold("USAGE") + "\n")
	rows := [][2]string{
		{"skill add <source>", "install a skill (owner/repo[/path], git URL, or local path)"},
		{"skill list", "list installed local skills"},
		{"skill remove <name>", "remove an installed skill"},
	}
	for _, r := range rows {
		b.WriteString("  " + cyan(pad(r[0], 24)) + dim(r[1]) + "\n")
	}
	b.WriteString("\n" + bold("EXAMPLES") + "\n")
	b.WriteString("  " + cyan("pilot skill add acme/cool-skill") + "\n")
	b.WriteString("  " + cyan("pilot skill add acme/skills/pdf-writer#main") + "\n")
	b.WriteString("  " + cyan("pilot skill add https://github.com/acme/skills.git#v1") + "\n")
	b.WriteString("  " + cyan("pilot skill add ./my-skill") + "\n\n")
	b.WriteString(dim("installs to: "+localSkillsDir()) + "\n")
	return b.String()
}
