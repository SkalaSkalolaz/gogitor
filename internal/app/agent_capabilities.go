package app

import (
	"sort"
	"strings"

	"gogitor/internal/config"
	"gogitor/internal/workspace"
)

type resolvedAgentCapabilities struct {
	Profile        modelProfile
	PreferredDepth AgentDepth
	ContextTokens  int
	PatchPolicy    workspace.PatchPolicy
	HasPatchPolicy bool
	MaxSubtasks    int
}

func (s *Service) configuredAgentCapability() (
	config.AgentModelCapability,
	bool,
) {
	if s.Cfg == nil ||
		len(s.Cfg.AgentModelCapabilities) == 0 {
		return config.AgentModelCapability{}, false
	}

	provider :=
		strings.ToLower(
			strings.TrimSpace(
				s.Cfg.Provider,
			),
		)

	model :=
		strings.ToLower(
			strings.TrimSpace(
				s.Cfg.Model,
			),
		)

	candidates := []string{
		model,
		provider + "/" + model,
		provider,
	}

	for _, key := range candidates {
		if key == "/" || key == "" {
			continue
		}

		if capability, ok :=
			s.Cfg.AgentModelCapabilities[key]; ok {
			return capability, true
		}
	}

	type candidate struct {
		key string
		cap config.AgentModelCapability
	}

	var matches []candidate

	for key, cap := range s.Cfg.AgentModelCapabilities {

		normalized := strings.ToLower(
			strings.TrimSpace(key),
		)

		if normalized == "" ||
			strings.Contains(
				normalized,
				"/",
			) {
			continue
		}

		if strings.Contains(
			model,
			normalized,
		) {
			matches = append(
				matches,
				candidate{
					key: normalized,
					cap: cap,
				},
			)
		}
	}

	if len(matches) == 0 {
		return config.AgentModelCapability{}, false
	}

	sort.Slice(
		matches,
		func(i, j int) bool {
			if len(matches[i].key) !=
				len(matches[j].key) {
				return len(matches[i].key) >
					len(matches[j].key)
			}

			return matches[i].key <
				matches[j].key
		},
	)

	return matches[0].cap, true
}

func (s *Service) agentModelCapabilities() resolvedAgentCapabilities {
	result := resolvedAgentCapabilities{
		Profile:        s.modelProfile(),
		PreferredDepth: AgentDepthAuto,
	}

	override, ok :=
		s.configuredAgentCapability()

	if !ok {
		return result
	}

	switch strings.ToLower(
		strings.TrimSpace(
			override.Profile,
		),
	) {
	case "small":
		result.Profile = modelProfileSmall

	case "medium":
		result.Profile = modelProfileMedium

	case "large":
		result.Profile = modelProfileLarge
	}

	depth :=
		normalizeAgentDepth(
			override.PreferredDepth,
		)

	if depth != AgentDepthAuto {
		result.PreferredDepth = depth
	}

	if override.ContextTokens > 0 {
		result.ContextTokens =
			override.ContextTokens
	}

	if policy, valid :=
		workspace.ParsePatchPolicy(
			override.PatchPolicy,
		); valid {

		result.PatchPolicy = policy
		result.HasPatchPolicy = true
	}

	if override.MaxSubtasks > 0 {
		result.MaxSubtasks =
			override.MaxSubtasks

		if result.MaxSubtasks > 7 {
			result.MaxSubtasks = 7
		}
	}

	return result
}

func (s *Service) effectiveAgentContextTokens() int {
	if s.Cfg.MaxContextTokens > 0 {
		return s.Cfg.MaxContextTokens
	}

	caps := s.agentModelCapabilities()

	if caps.ContextTokens > 0 {
		return caps.ContextTokens
	}

	return s.Cfg.EffectiveContextTokens()
}
