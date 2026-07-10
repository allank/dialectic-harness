// Package runstore owns the on-disk layout: hidden machine state in
// .a2a/<slug>-<timestamp>/ beside the artifact, human outputs as portable
// Markdown beside the artifact.
package runstore

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/allank/dialectic/internal/state"
	"gopkg.in/yaml.v3"
)

const timestampLayout = "20060102T150405"

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(artifactPath string) string {
	base := filepath.Base(artifactPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	s := nonAlnum.ReplaceAllString(strings.ToLower(base), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "run"
	}
	return s
}

type RunPaths struct {
	RunDir      string
	StatePath   string
	TurnsDir    string
	ScratchDir  string
	SummaryPath string
	BriefPath   string
}

func NewRun(artifactPath string, now time.Time) (RunPaths, error) {
	dir := filepath.Dir(artifactPath)
	slug := Slug(artifactPath)
	stamp := now.Format(timestampLayout)
	runDir := filepath.Join(dir, ".a2a", fmt.Sprintf("%s-%s", slug, stamp))
	p := RunPaths{
		RunDir:      runDir,
		StatePath:   filepath.Join(runDir, "debate-state.yaml"),
		TurnsDir:    filepath.Join(runDir, "turns"),
		ScratchDir:  filepath.Join(runDir, "scratch"),
		SummaryPath: filepath.Join(dir, fmt.Sprintf("%s-debate-summary-%s.md", slug, stamp)),
		BriefPath:   filepath.Join(dir, fmt.Sprintf("%s-update-brief-%s.md", slug, stamp)),
	}
	for _, d := range []string{p.TurnsDir, p.ScratchDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return RunPaths{}, fmt.Errorf("create run dir: %w", err)
		}
	}
	return p, nil
}

func WriteSummary(p RunPaths, summary string) error {
	if err := os.WriteFile(p.SummaryPath, []byte(summary), 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

// briefFrontmatter is the YAML frontmatter prepended to an update brief.
// Marshaled via yaml.Marshal so that scalars needing quoting (colons,
// quotes, leading/trailing whitespace, etc.) are escaped correctly.
type briefFrontmatter struct {
	ArbiterVerdict string `yaml:"arbiter_verdict"`
	VerdictWhy     string `yaml:"verdict_why"`
	TopicSlug      string `yaml:"topic_slug"`
	TargetArtifact string `yaml:"target_artifact"`
	Outcome        string `yaml:"outcome"`
	RunDir         string `yaml:"run_dir"`
	Created        string `yaml:"created"`
}

// WriteUpdateBrief writes the narrative + update brief as one note with the
// kill-criterion frontmatter. Allan flips arbiter_verdict after acting.
func WriteUpdateBrief(p RunPaths, st *state.DebateState, compilerDoc, outcome string, now time.Time) error {
	relRun, err := filepath.Rel(filepath.Dir(p.BriefPath), p.RunDir)
	if err != nil {
		relRun = p.RunDir
	}
	fm := briefFrontmatter{
		ArbiterVerdict: "pending",
		VerdictWhy:     "",
		TopicSlug:      st.TopicSlug,
		TargetArtifact: filepath.Base(st.TargetArtifact),
		Outcome:        outcome,
		RunDir:         relRun,
		Created:        now.Format("2006-01-02"),
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal brief frontmatter: %w", err)
	}
	doc := "---\n" + string(fmBytes) + "---\n\n" + compilerDoc
	if err := os.WriteFile(p.BriefPath, []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write update brief: %w", err)
	}
	return nil
}
