package spec

import "github.com/goccy/go-yaml"

// Spec is the YAML build specification (melange-style).
type Spec struct {
	Name         string            `yaml:"name" json:"name"`
	Version      string            `yaml:"version" json:"version"`
	Epoch        int               `yaml:"epoch,omitempty" json:"epoch,omitempty"` // used as pkgrel in APKBUILD (default 0)
	URL          string            `yaml:"url,omitempty" json:"url,omitempty"`
	License      string            `yaml:"license,omitempty" json:"license,omitempty"`
	Description  string            `yaml:"description" json:"description,omitempty"`
	Copyright    []Copyright       `yaml:"copyright,omitempty" json:"copyright,omitempty"`
	Dependencies Dependencies      `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Environment  Environment       `yaml:"environment,omitempty" json:"environment,omitempty"`
	Sources      map[string]Source `yaml:"sources,omitempty" json:"sources,omitempty"`
	Pipeline     []PipelineStep    `yaml:"pipeline" json:"pipeline"`
	Build        Build             `yaml:"build,omitempty" json:"build,omitempty"` // optional install_dir, source_dir
}

// Copyright entry (e.g. attestation + license).
type Copyright struct {
	Attestation string `yaml:"attestation" json:"attestation"`
	License     string `yaml:"license" json:"license"`
}

// Dependencies declares package dependencies (runtime, etc.) for the produced APK.
type Dependencies struct {
	Runtime []string `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}

// Environment defines the build environment (repositories + packages to install).
type Environment struct {
	Contents EnvironmentContents `yaml:"contents" json:"contents"`
}

// EnvironmentContents lists repositories and packages for the build environment.
type EnvironmentContents struct {
	Repositories []string `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	Packages     []string `yaml:"packages,omitempty" json:"packages,omitempty"`
}

// Source defines a single source (e.g. from build context).
type Source struct {
	Context *SourceContext `yaml:"context,omitempty" json:"context,omitempty"`
}

// SourceContext uses the Docker build context.
type SourceContext struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
}

// Build holds optional install prefix and source subdir (pipeline is top-level).
type Build struct {
	InstallDir string `yaml:"install_dir,omitempty" json:"install_dir,omitempty"`
	SourceDir  string `yaml:"source_dir,omitempty" json:"source_dir,omitempty"`
}

// PipelineStep is one step in the build pipeline: either "uses" (predefined) or "run" (inline).
type PipelineStep struct {
	Uses string                 `yaml:"uses,omitempty" json:"uses,omitempty"`
	With map[string]interface{} `yaml:"with,omitempty" json:"with,omitempty"`
	Run  string                 `yaml:"run,omitempty" json:"run,omitempty"`
}

// Load parses YAML bytes into Spec.
func Load(data []byte) (*Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Build.InstallDir == "" {
		s.Build.InstallDir = "/usr"
	}
	return &s, nil
}
