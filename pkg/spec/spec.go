package spec

import "github.com/goccy/go-yaml"

// Spec is the YAML build specification for cmake-apk.
type Spec struct {
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version" json:"version"`
	Epoch       string            `yaml:"epoch,omitempty" json:"epoch,omitempty"` // used as pkgrel in APKBUILD (default "0")
	URL         string            `yaml:"url,omitempty" json:"url,omitempty"`
	License     string            `yaml:"license,omitempty" json:"license,omitempty"`
	Description string            `yaml:"description" json:"description,omitempty"`
	Sources     map[string]Source `yaml:"sources" json:"sources,omitempty"`
	Build       Build             `yaml:"build" json:"build"`
}

// Source defines a single source (e.g. from build context).
type Source struct {
	Context *SourceContext `yaml:"context,omitempty" json:"context,omitempty"`
}

// SourceContext uses the Docker build context.
type SourceContext struct {
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
}

// Build defines how to build (cmake) and optional install prefix.
type Build struct {
	Steps      []string `yaml:"steps,omitempty" json:"steps,omitempty"`
	InstallDir string   `yaml:"install_dir,omitempty" json:"install_dir,omitempty"`
	SourceDir  string   `yaml:"source_dir,omitempty" json:"source_dir,omitempty"`
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
	if s.Epoch == "" {
		s.Epoch = "0"
	}
	return &s, nil
}
