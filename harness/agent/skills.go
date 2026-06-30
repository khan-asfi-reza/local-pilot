package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// skillSet is the result of scanning the skills directory: a cheap catalog to
// keep resident, the full bodies to inject on demand, and the names for the
// load_skill enum.
type skillSet struct {
	catalog string
	bodies  map[string]string
	names   []string
}

// scanSkills reads each skill folder's SKILL.md, taking the name and description
// from the frontmatter for the catalog and keeping the full body for load_skill.
func scanSkills(dir string) (skillSet, error) {
	set := skillSet{bodies: map[string]string{}}
	if dir == "" {
		return set, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A missing skills directory is not an error; there are simply no skills.
		if os.IsNotExist(err) {
			return set, nil
		}
		return set, err
	}
	var lines []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name, desc := parseFrontmatter(string(raw))
		if name == "" {
			name = e.Name()
		}
		set.names = append(set.names, name)
		set.bodies[name] = string(raw)
		lines = append(lines, "- "+name+": "+desc)
	}
	set.catalog = strings.Join(lines, "\n")
	return set, nil
}

// parseFrontmatter pulls name and description out of a leading YAML frontmatter
// block delimited by lines of three dashes.
func parseFrontmatter(text string) (name, desc string) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
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
	}
	return name, desc
}

func frontValue(line, key string) (string, bool) {
	prefix := key + ":"
	if strings.HasPrefix(strings.TrimSpace(line), prefix) {
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix)), true
	}
	return "", false
}
