package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skills (Ep04). A skill is a folder with a SKILL.md, installed unmodified. The harness shows
// the model each skill's name and description in the system prompt, the way Claude Code
// does, and gives it a Skill tool that loads a skill's SKILL.md (and any file inside the
// skill's folder) when the model decides it applies.

// Skill is one installed skill.
type Skill struct {
	Name        string
	Description string
	Dir         string // absolute path of the skill's folder
}

// skipDirs are never searched for skills: repository plumbing and test fixtures, which some
// packs ship with SKILL.md files that aren't real skills.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "tests": true, "test": true, "fixtures": true, "testdata": true}

// LoadSkills finds the skills under each path. A path is a skill folder (it holds SKILL.md)
// or a pack (a folder with skills somewhere inside, like Superpowers). Skills marked
// `disable-model-invocation: true` are left out: Claude Code doesn't show those to the model
// either, only a person can start them.
func LoadSkills(paths []string) ([]Skill, error) {
	var skills []Skill
	seen := map[string]string{}
	for _, root := range paths {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		found := 0
		err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != abs && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() != "SKILL.md" {
				return nil
			}
			meta, err := readFrontmatter(p)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			if meta["disable-model-invocation"] == "true" {
				return nil
			}
			name, desc := meta["name"], meta["description"]
			if name == "" {
				name = filepath.Base(filepath.Dir(p))
			}
			if desc == "" {
				return fmt.Errorf("%s: no description in its frontmatter", p)
			}
			if other, dup := seen[name]; dup {
				return fmt.Errorf("two skills are named %q: %s and %s", name, other, filepath.Dir(p))
			}
			seen[name] = filepath.Dir(p)
			skills = append(skills, Skill{Name: name, Description: desc, Dir: filepath.Dir(p)})
			found++
			return nil
		})
		if err != nil {
			return nil, err
		}
		if found == 0 {
			return nil, fmt.Errorf("no SKILL.md under %s", root)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// readFrontmatter reads the simple YAML block at the top of a SKILL.md: `key: value` lines,
// quoted values, and folded or literal blocks (`key: >` / `key: |` with indented lines).
// Nested keys (metadata:, hooks:) are skipped. Only the top-level scalars are needed.
func readFrontmatter(path string) (map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 -- a skill folder the person running the harness chose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return nil, errors.New("SKILL.md doesn't start with a --- frontmatter block")
	}
	meta := map[string]string{}
	var blockKey string
	var block []string
	flush := func() {
		if blockKey != "" {
			meta[blockKey] = strings.Join(block, " ")
		}
		blockKey, block = "", nil
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			flush()
			return meta, nil
		}
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if indented {
			if blockKey != "" && strings.TrimSpace(line) != "" {
				block = append(block, strings.TrimSpace(line))
			}
			continue
		}
		flush()
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case value == ">" || value == "|" || value == ">-" || value == "|-":
			blockKey = key
		case len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\''):
			meta[key] = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
		default:
			meta[key] = value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("frontmatter block is never closed with ---")
}

// skillsPrompt is the block added to the system prompt: every skill's name and description,
// in the same shape Claude Code uses. Superpowers' plugin also injects its
// using-superpowers skill at session start with a hook, so when that skill is installed
// its full text goes in too, wrapped exactly as the plugin's hook wraps it.
func skillsPrompt(skills []Skill) (string, error) {
	if len(skills) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("\n\nThe following skills are available for use with the Skill tool:\n\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	for _, s := range skills {
		if s.Name != "using-superpowers" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Dir, "SKILL.md")) // #nosec G304 -- a chosen skill folder
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n<EXTREMELY_IMPORTANT>\nYou have superpowers.\n\n**Below is the full content of your 'superpowers:using-superpowers' skill - your introduction to using skills. For all other skills, use the 'Skill' tool:**\n\n%s\n</EXTREMELY_IMPORTANT>\n", strings.TrimSpace(string(data)))
	}
	return b.String(), nil
}

// loadSkill is the Skill tool: a skill's SKILL.md, or with file set, another file inside
// that skill's folder (a reference the SKILL.md points to). Nothing outside the folder.
func loadSkill(skills []Skill, name, file string) (string, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "superpowers:") // how Superpowers refers to its own skills
	var skill *Skill
	for i := range skills {
		if skills[i].Name == name {
			skill = &skills[i]
		}
	}
	if skill == nil {
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.Name
		}
		return "", fmt.Errorf("no skill named %q; installed: %s", name, strings.Join(names, ", "))
	}
	if file == "" {
		data, err := os.ReadFile(filepath.Join(skill.Dir, "SKILL.md")) // #nosec G304 -- inside a chosen skill folder
		if err != nil {
			return "", err
		}
		others := skillFiles(skill.Dir)
		note := ""
		if len(others) > 0 {
			note = "\n\nOther files in this skill (load one with the Skill tool's file argument): " + strings.Join(others, ", ")
		}
		return fmt.Sprintf("Skill %q loaded.\n\n%s%s", skill.Name, string(data), note), nil
	}
	if filepath.IsAbs(file) {
		return "", fmt.Errorf("file %q must be relative to the skill's folder", file)
	}
	full := filepath.Join(skill.Dir, file)
	if real, err := filepath.EvalSymlinks(full); err == nil {
		full = real
	}
	rel, err := filepath.Rel(skill.Dir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file %q is outside the skill's folder", file)
	}
	data, err := os.ReadFile(full) // #nosec G304 -- checked to be inside the skill folder
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// skillFiles lists a skill's other files, relative to its folder.
func skillFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "SKILL.md" {
			return nil
		}
		if rel, err := filepath.Rel(dir, p); err == nil && len(files) < 50 {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files
}
