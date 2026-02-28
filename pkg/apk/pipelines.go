package apk

import (
	"embed"
	"fmt"
	"sync"

	"github.com/goccy/go-yaml"
)

//go:embed pipelines/*.yaml pipelines/*/*.yaml
var pipelinesFS embed.FS

// InputDef describes one pipeline input (melange-style): description, optional default, required.
// In YAML, an input can be a string (default value) or an object: { description?, default?, required? }
type InputDef struct {
	Description string
	Default     string
	Required    bool
}

// UnmarshalYAML supports short form (string = default) or long form (object with description, default, required).
func (i *InputDef) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		i.Default = s
		return nil
	}
	var m struct {
		Description string `yaml:"description"`
		Default     string `yaml:"default"`
		Required    bool   `yaml:"required"`
	}
	if err := unmarshal(&m); err != nil {
		return err
	}
	i.Description = m.Description
	i.Default = m.Default
	i.Required = m.Required
	return nil
}

// PipelineNeeds declares what a pipeline needs (e.g. packages to install in the build environment).
type PipelineNeeds struct {
	Packages []string `yaml:"packages,omitempty"`
}

// PipelineDef is the structure of a pipeline YAML file.
type PipelineDef struct {
	Name   string             `yaml:"name,omitempty"`
	Needs  PipelineNeeds      `yaml:"needs,omitempty"`
	Inputs map[string]InputDef `yaml:"inputs,omitempty"` // input name -> schema (default, required)
	Runs   string             `yaml:"runs,omitempty"`
}

var (
	loadedPipelines   map[string]*PipelineDef
	loadedPipelinesMu sync.Mutex
)

// getPipeline loads and returns the pipeline definition for the given name (e.g. "fetch", "autoconf/configure").
func getPipeline(name string) (*PipelineDef, error) {
	loadedPipelinesMu.Lock()
	defer loadedPipelinesMu.Unlock()
	if loadedPipelines == nil {
		loadedPipelines = make(map[string]*PipelineDef)
	}
	if def, ok := loadedPipelines[name]; ok {
		return def, nil
	}
	path := name + ".yaml"
	data, err := pipelinesFS.ReadFile("pipelines/" + path)
	if err != nil {
		return nil, fmt.Errorf("pipeline %q not found: %w", name, err)
	}
	var def PipelineDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("pipeline %q: %w", name, err)
	}
	if def.Runs == "" {
		return nil, fmt.Errorf("pipeline %q: missing runs", name)
	}
	if def.Inputs == nil {
		def.Inputs = make(map[string]InputDef)
	}
	loadedPipelines[name] = &def
	return &def, nil
}
