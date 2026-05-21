package main

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var profileFS embed.FS

type TracingProfile struct {
	Name           string                  `yaml:"name,omitempty" json:"name,omitempty"`
	Hidden         bool                    `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Format         string                  `yaml:"format,omitempty" json:"format,omitempty"`
	Index          string                  `yaml:"index,omitempty" json:"index,omitempty"`
	DateField      string                  `yaml:"dateField,omitempty" json:"dateField,omitempty"`
	TraceIDField   string                  `yaml:"traceIdField,omitempty" json:"traceIdField,omitempty"`
	SpanIDField    string                  `yaml:"spanIdField,omitempty" json:"spanIdField,omitempty"`
	ParentIDField  string                  `yaml:"parentIdField,omitempty" json:"parentIdField,omitempty"`
	ParentRefType  string                  `yaml:"parentRefType,omitempty" json:"parentRefType,omitempty"`
	ServiceField   string                  `yaml:"serviceField,omitempty" json:"serviceField,omitempty"`
	OperationField string                  `yaml:"operationField,omitempty" json:"operationField,omitempty"`
	StatusFields   []string                `yaml:"statusFields,omitempty" json:"statusFields,omitempty"`
	SelectFields   []string                `yaml:"selectFields,omitempty" json:"selectFields,omitempty"`
	SourceExcludes []string                `yaml:"sourceExcludes,omitempty" json:"sourceExcludes,omitempty"`
	Imports        []string                `yaml:"imports,omitempty" json:"imports,omitempty"`
	Defaults       map[string]any          `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Params         map[string]TracingParam `yaml:"params,omitempty" json:"params,omitempty"`
	Columns        []TracingColumn         `yaml:"columns,omitempty" json:"columns,omitempty"`
}

type TracingParam struct {
	Field       string `yaml:"field,omitempty" json:"field,omitempty"`
	Operator    string `yaml:"operator,omitempty" json:"operator,omitempty"`
	Clause      string `yaml:"clause,omitempty" json:"clause,omitempty"`
	Format      string `yaml:"format,omitempty" json:"format,omitempty"`
	Template    string `yaml:"template,omitempty" json:"template,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Internal    bool   `yaml:"internal,omitempty" json:"internal,omitempty"`
}

type TracingColumn struct {
	Name   string `yaml:"name,omitempty" json:"name,omitempty"`
	Field  string `yaml:"field,omitempty" json:"field,omitempty"`
	Detail bool   `yaml:"detail,omitempty" json:"detail,omitempty"`
}

type ProfileSummary struct {
	Name     string          `json:"name"`
	Format   string          `json:"format"`
	Backend  string          `json:"backend"`
	Index    string          `json:"index"`
	Params   []ProfileParam  `json:"params"`
	Columns  []TracingColumn `json:"columns"`
	Defaults map[string]any  `json:"defaults"`
	Profile  *TracingProfile `json:"-"`
}

type ProfileParam struct {
	Name        string `json:"name"`
	Operator    string `json:"operator"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

func defaultProfiles() map[string]TracingProfile {
	entries, err := fs.ReadDir(profileFS, "profiles")
	if err != nil {
		panic(err)
	}
	out := make(map[string]TracingProfile, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := profileFS.ReadFile(path.Join("profiles", entry.Name()))
		if err != nil {
			panic(err)
		}
		var p TracingProfile
		if err := yaml.Unmarshal(data, &p); err != nil {
			panic(err)
		}
		if p.Name == "" {
			p.Name = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		out[p.Name] = p
	}
	return out
}

func parseProfilesYAML(data []byte) (map[string]TracingProfile, error) {
	var profiles map[string]TracingProfile
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		var single TracingProfile
		if err2 := yaml.Unmarshal(data, &single); err2 != nil {
			return nil, fmt.Errorf("parse profilesYaml: %w", err)
		}
		if single.Name == "" {
			return nil, fmt.Errorf("profilesYaml single profile requires name")
		}
		profiles = map[string]TracingProfile{single.Name: single}
	}
	for name, profile := range profiles {
		if profile.Name == "" {
			profile.Name = name
			profiles[name] = profile
		}
	}
	return profiles, nil
}

func (s PluginSettings) resolveProfile(name string) (TracingProfile, error) {
	if strings.TrimSpace(name) == "" {
		name = s.DefaultProfile
	}
	defaults := defaultProfiles()
	custom := s.Profiles
	resolved := map[string]TracingProfile{}
	visiting := map[string]bool{}

	var resolve func(string) (TracingProfile, error)
	resolve = func(profileName string) (TracingProfile, error) {
		if p, ok := resolved[profileName]; ok {
			return p, nil
		}
		if visiting[profileName] {
			return TracingProfile{}, fmt.Errorf("cyclic tracing profile import at %q", profileName)
		}
		visiting[profileName] = true
		defer delete(visiting, profileName)

		base, hasDefault := defaults[profileName]
		override, hasCustom := custom[profileName]
		if !hasDefault && !hasCustom {
			return TracingProfile{}, fmt.Errorf("tracing profile %q not found", profileName)
		}

		profile := TracingProfile{Name: profileName}
		for _, imported := range base.Imports {
			p, err := resolve(imported)
			if err != nil {
				return TracingProfile{}, fmt.Errorf("resolve import %q for %q: %w", imported, profileName, err)
			}
			profile = mergeProfiles(profile, p)
		}
		profile = mergeProfiles(profile, base)
		for _, imported := range override.Imports {
			p, err := resolve(imported)
			if err != nil {
				return TracingProfile{}, fmt.Errorf("resolve import %q for %q: %w", imported, profileName, err)
			}
			profile = mergeProfiles(profile, p)
		}
		profile = mergeProfiles(profile, override)
		if profile.Name == "" {
			profile.Name = profileName
		}
		if s.Index != "" {
			profile.Index = s.Index
		}
		profile = profile.withDefaults()
		if err := profile.validate(); err != nil {
			return TracingProfile{}, err
		}
		resolved[profileName] = profile
		return profile, nil
	}
	return resolve(name)
}

func (s PluginSettings) profileNames() []string {
	names := map[string]struct{}{}
	for name := range defaultProfiles() {
		names[name] = struct{}{}
	}
	for name := range s.Profiles {
		names[name] = struct{}{}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		p, err := s.resolveProfile(name)
		if err == nil && !p.Hidden {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		leftDepth := strings.Count(out[i], ".")
		rightDepth := strings.Count(out[j], ".")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return out[i] < out[j]
	})
	return out
}

func (s PluginSettings) profileSummaries() ([]ProfileSummary, error) {
	names := s.profileNames()
	out := make([]ProfileSummary, 0, len(names))
	for _, name := range names {
		profile, err := s.resolveProfile(name)
		if err != nil {
			return nil, err
		}
		params := make([]ProfileParam, 0, len(profile.Params))
		paramNames := make([]string, 0, len(profile.Params))
		for name, param := range profile.Params {
			if !param.Internal {
				paramNames = append(paramNames, name)
			}
		}
		sort.Strings(paramNames)
		for _, name := range paramNames {
			param := profile.Params[name]
			params = append(params, ProfileParam{
				Name:        name,
				Operator:    firstNonEmpty(param.Operator, "term"),
				Description: param.Description,
				Required:    param.Required,
			})
		}
		p := profile
		out = append(out, ProfileSummary{
			Name:     profile.Name,
			Format:   profile.Format,
			Backend:  "opensearch",
			Index:    profile.Index,
			Params:   params,
			Columns:  profile.Columns,
			Defaults: cloneMap(profile.Defaults),
			Profile:  &p,
		})
	}
	return out, nil
}

func (p TracingProfile) withDefaults() TracingProfile {
	if p.Format == "" {
		p.Format = "flat"
	}
	if p.Index == "" {
		p.Index = "otel-traces-*"
	}
	if p.DateField == "" {
		p.DateField = "@timestamp"
	}
	if p.TraceIDField == "" {
		p.TraceIDField = "trace_id"
	}
	if p.SpanIDField == "" {
		p.SpanIDField = "span_id"
	}
	if p.ParentIDField == "" {
		p.ParentIDField = "parent_id"
	}
	if p.ServiceField == "" {
		p.ServiceField = "service_name"
	}
	if p.OperationField == "" {
		p.OperationField = "operation_name"
	}
	if p.Params == nil {
		p.Params = map[string]TracingParam{}
	}
	if p.Defaults == nil {
		p.Defaults = map[string]any{}
	}
	if len(p.Columns) == 0 {
		p.Columns = []TracingColumn{
			{Name: "Time", Field: "timestamp"},
			{Name: "Service", Field: "service_name"},
			{Name: "Operation", Field: "operation_name"},
			{Name: "Status", Field: "status"},
			{Name: "Trace ID", Field: "trace_id"},
		}
	}
	return p
}

func (p TracingProfile) validate() error {
	if p.Name == "" {
		return fmt.Errorf("tracing profile name is required")
	}
	if p.Index == "" {
		return fmt.Errorf("tracing profile %q index is required", p.Name)
	}
	if p.DateField == "" {
		return fmt.Errorf("tracing profile %q dateField is required", p.Name)
	}
	for name, param := range p.Params {
		if param.Field == "" {
			return fmt.Errorf("tracing profile %q param %q field is required", p.Name, name)
		}
	}
	return nil
}

func mergeProfiles(base, override TracingProfile) TracingProfile {
	out := base
	if override.Name != "" {
		out.Name = override.Name
	}
	if override.Hidden {
		out.Hidden = true
	}
	if override.Format != "" {
		out.Format = override.Format
	}
	if override.Index != "" {
		out.Index = override.Index
	}
	if override.DateField != "" {
		out.DateField = override.DateField
	}
	if override.TraceIDField != "" {
		out.TraceIDField = override.TraceIDField
	}
	if override.SpanIDField != "" {
		out.SpanIDField = override.SpanIDField
	}
	if override.ParentIDField != "" {
		out.ParentIDField = override.ParentIDField
	}
	if override.ParentRefType != "" {
		out.ParentRefType = override.ParentRefType
	}
	if override.ServiceField != "" {
		out.ServiceField = override.ServiceField
	}
	if override.OperationField != "" {
		out.OperationField = override.OperationField
	}
	if len(override.StatusFields) > 0 {
		out.StatusFields = append([]string(nil), override.StatusFields...)
	}
	if len(override.SelectFields) > 0 {
		out.SelectFields = append([]string(nil), override.SelectFields...)
	}
	if len(override.SourceExcludes) > 0 {
		out.SourceExcludes = append([]string(nil), override.SourceExcludes...)
	}
	if len(override.Columns) > 0 {
		out.Columns = append([]TracingColumn(nil), override.Columns...)
	}
	if out.Defaults == nil {
		out.Defaults = map[string]any{}
	}
	for k, v := range override.Defaults {
		out.Defaults[k] = v
	}
	if out.Params == nil {
		out.Params = map[string]TracingParam{}
	}
	for k, v := range override.Params {
		out.Params[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
