// Package state defines the debate-state schema: the single YAML document
// the orchestrator owns. Agents never write this file; they submit turn
// files that the engine merges in.
package state

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Role string

const (
	RoleChallenger Role = "challenger"
	RoleIncumbent  Role = "incumbent"
)

func (r Role) Other() Role {
	if r == RoleChallenger {
		return RoleIncumbent
	}
	return RoleChallenger
}

type Stance string

const (
	StanceConcur   Stance = "concur"
	StanceRebut    Stance = "rebut"
	StanceWithdraw Stance = "withdraw"
	StanceNew      Stance = "new"
)

func ValidStance(s Stance) bool {
	switch s {
	case StanceConcur, StanceRebut, StanceWithdraw, StanceNew:
		return true
	}
	return false
}

type Contention struct {
	ID       string          `yaml:"id"`
	Issue    string          `yaml:"issue"`
	Severity string          `yaml:"severity,omitempty"`
	Stances  map[Role]string `yaml:"stances,omitempty"`
}

type ConsensusItem struct {
	ID           string `yaml:"id"`
	Issue        string `yaml:"issue"`
	ResolvedTurn int    `yaml:"resolved_turn"`
	Rationale    string `yaml:"rationale"`
}

type LedgerEntry struct {
	Turn       int    `yaml:"turn"`
	Agent      Role   `yaml:"agent"`
	Contention string `yaml:"contention"`
	Stance     Stance `yaml:"stance"`
	Rationale  string `yaml:"rationale"`
}

type Directive struct {
	Target     Role   `yaml:"target"`
	Contention string `yaml:"contention"`
	Directive  string `yaml:"directive"`
	IssuedTurn int    `yaml:"issued_turn"`
}

type DebateState struct {
	TopicSlug         string          `yaml:"topic_slug"`
	TargetArtifact    string          `yaml:"target_artifact"`
	TurnCount         int             `yaml:"turn_count"`
	RoundCount        int             `yaml:"round_count"`
	MaxRounds         int             `yaml:"max_rounds"`
	NextRole          Role            `yaml:"next_role"`
	ContentionCounter int             `yaml:"contention_counter"`
	Roles             map[Role]string `yaml:"roles"`
	Sessions          map[Role]string `yaml:"sessions"`
	ConsensusBaseline []ConsensusItem `yaml:"consensus_baseline"`
	Withdrawn         []ConsensusItem `yaml:"withdrawn,omitempty"`
	ActiveContentions []Contention    `yaml:"active_contentions"`
	ContentionHistory []LedgerEntry   `yaml:"contention_history"`
	RequiredRebuttals []Directive     `yaml:"required_rebuttals"`
	IgnoredDirectives []Directive     `yaml:"ignored_directives,omitempty"`
}

func New(topicSlug, targetArtifact string, maxRounds int, roles map[Role]string) *DebateState {
	if roles == nil {
		roles = map[Role]string{}
	}
	return &DebateState{
		TopicSlug:         topicSlug,
		TargetArtifact:    targetArtifact,
		MaxRounds:         maxRounds,
		NextRole:          RoleChallenger,
		Roles:             roles,
		Sessions:          map[Role]string{},
		ConsensusBaseline: []ConsensusItem{},
		Withdrawn:         []ConsensusItem{},
		ActiveContentions: []Contention{},
		ContentionHistory: []LedgerEntry{},
		RequiredRebuttals: []Directive{},
		IgnoredDirectives: []Directive{},
	}
}

func Load(path string) (*DebateState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	st := &DebateState{}
	if err := yaml.Unmarshal(raw, st); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if st.Sessions == nil {
		st.Sessions = map[Role]string{}
	}
	if st.Roles == nil {
		st.Roles = map[Role]string{}
	}
	if st.ConsensusBaseline == nil {
		st.ConsensusBaseline = []ConsensusItem{}
	}
	if st.Withdrawn == nil {
		st.Withdrawn = []ConsensusItem{}
	}
	if st.ActiveContentions == nil {
		st.ActiveContentions = []Contention{}
	}
	if st.ContentionHistory == nil {
		st.ContentionHistory = []LedgerEntry{}
	}
	if st.RequiredRebuttals == nil {
		st.RequiredRebuttals = []Directive{}
	}
	if st.IgnoredDirectives == nil {
		st.IgnoredDirectives = []Directive{}
	}
	return st, nil
}

func (st *DebateState) Save(path string) error {
	raw, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(path, raw, 0o644)
}

func (st *DebateState) NextContentionID() string {
	st.ContentionCounter++
	return fmt.Sprintf("C%d", st.ContentionCounter)
}

func (st *DebateState) FindActive(id string) *Contention {
	for i := range st.ActiveContentions {
		if st.ActiveContentions[i].ID == id {
			return &st.ActiveContentions[i]
		}
	}
	return nil
}

func (st *DebateState) IsConsensus(id string) bool {
	for _, c := range st.ConsensusBaseline {
		if c.ID == id {
			return true
		}
	}
	return false
}
