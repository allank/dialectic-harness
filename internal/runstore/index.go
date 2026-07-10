package runstore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type briefRow struct {
	Created  string
	Artifact string
	Slug     string
	Outcome  string
	Verdict  string
	Why      string
	RelPath  string
}

// BuildIndex regenerates the kill-criterion table: one row per update brief
// found under root, newest first. Plain Markdown; no Obsidian tooling needed.
func BuildIndex(root string) (string, error) {
	var rows []briefRow
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		fm := readFrontmatter(path)
		if fm["arbiter_verdict"] == "" {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		rows = append(rows, briefRow{
			Created:  fm["created"],
			Artifact: fm["target_artifact"],
			Slug:     fm["topic_slug"],
			Outcome:  fm["outcome"],
			Verdict:  fm["arbiter_verdict"],
			Why:      fm["verdict_why"],
			RelPath:  filepath.ToSlash(rel),
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Created != rows[j].Created {
			return rows[i].Created > rows[j].Created
		}
		return rows[i].RelPath < rows[j].RelPath
	})
	var b strings.Builder
	b.WriteString("# A2A Debate Runs\n\n")
	b.WriteString("| Created | Artifact | Slug | Outcome | Verdict | Why | Brief |\n")
	b.WriteString("|---------|----------|------|---------|---------|-----|-------|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | [brief](%s) |\n",
			r.Created, r.Artifact, r.Slug, r.Outcome, r.Verdict, r.Why, r.RelPath)
	}
	return b.String(), nil
}

// readFrontmatter parses the leading YAML frontmatter block as flat
// key: value pairs. Missing or malformed frontmatter yields an empty map.
func readFrontmatter(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}
