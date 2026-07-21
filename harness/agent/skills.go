package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// skillSet is the result of scanning the skills directory: a cheap catalog to
// keep resident, the full bodies to inject on demand, and the names for the
// load_skill enum. Internal skills are excluded from the catalog and enum — they
// are never surfaced to the model or shown as "skill loaded"; the harness injects
// their bodies silently when it detects the task needs them.
type skillSet struct {
	catalog  string
	bodies   map[string]string
	names    []string
	internal map[string]bool
}

// scanSkills reads each skill folder's SKILL.md across one or more directories,
// taking the name and description from the frontmatter for the catalog and
// keeping the full body for load_skill. Directories are scanned in order and a
// later one overrides an earlier one by name, so a user's local skill can shadow
// a default of the same name. Used to merge the shipped "default" skills with
// the user-installed "local" skills.
func scanSkills(dirs ...string) (skillSet, error) {
	set := skillSet{bodies: map[string]string{}, internal: map[string]bool{}}
	var lines []string
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A missing skills directory is not an error; there are simply none there.
			if os.IsNotExist(err) {
				continue
			}
			return set, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
			if err != nil {
				continue
			}
			name, desc, internal := parseFrontmatter(string(raw))
			if name == "" {
				name = e.Name()
			}
			first := set.bodies[name] == ""
			set.bodies[name] = string(raw)
			set.internal[name] = internal
			// Internal skills are injected silently by the harness, so they are kept
			// out of the visible catalog and the load_skill enum.
			if internal || !first {
				continue
			}
			set.names = append(set.names, name)
			lines = append(lines, "- "+name+": "+desc)
		}
	}
	set.catalog = strings.Join(lines, "\n")
	return set, nil
}

// parseFrontmatter pulls name, description, and the internal flag out of a
// leading YAML frontmatter block delimited by lines of three dashes.
func parseFrontmatter(text string) (name, desc string, internal bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			break
		}
		if v, ok := frontValue(ln, "name"); ok {
			name = v
		}
		if v, ok := frontValue(ln, "description"); ok {
			desc = v
		}
		if v, ok := frontValue(ln, "internal"); ok {
			internal = strings.EqualFold(strings.TrimSpace(v), "true")
		}
	}
	return name, desc, internal
}

func frontValue(line, key string) (string, bool) {
	prefix := key + ":"
	if strings.HasPrefix(strings.TrimSpace(line), prefix) {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix)), true
	}
	return "", false
}
