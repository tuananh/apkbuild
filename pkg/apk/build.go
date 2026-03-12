package apk

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/pkg/errors"
	"github.com/tuananh/apkbuild/pkg/spec"
)

const alpineImage = "alpine:3.23"

// collectPipelinePackages returns a deduplicated list of packages required by pipeline steps (from each pipeline's needs.packages).
func collectPipelinePackages(s *spec.Spec) ([]string, error) {
	seen := make(map[string]struct{})
	for _, step := range s.Pipeline {
		if step.Uses == "" {
			continue
		}
		def, err := getPipeline(step.Uses)
		if err != nil {
			return nil, err
		}
		for _, pkg := range def.Needs.Packages {
			seen[pkg] = struct{}{}
		}
	}
	list := make([]string, 0, len(seen))
	for pkg := range seen {
		list = append(list, pkg)
	}
	sort.Strings(list)
	return list, nil
}

// buildInstallCommand returns a shell script that configures apk repos (if any) and installs packages from the spec plus all packages needed by pipelines (deduplicated).
func buildInstallCommand(s *spec.Spec) (string, error) {
	pipelinePkgs, err := collectPipelinePackages(s)
	if err != nil {
		return "", err
	}
	seen := make(map[string]struct{})
	var all []string
	for _, p := range s.Environment.Contents.Packages {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			all = append(all, p)
		}
	}
	for _, p := range pipelinePkgs {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			all = append(all, p)
		}
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, repo := range s.Environment.Contents.Repositories {
		b.WriteString(fmt.Sprintf("echo %q >> /etc/apk/repositories\n", repo))
	}
	if len(all) > 0 {
		b.WriteString("apk add --no-cache ")
		b.WriteString(strings.Join(all, " "))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// validatePipelineStep checks that step.With conforms to the pipeline's input schema.
func validatePipelineStep(def *PipelineDef, step *spec.PipelineStep, stepIndex int) error {
	for key := range step.With {
		if _, ok := def.Inputs[key]; !ok {
			return fmt.Errorf("pipeline step %d (%s): unknown input %q (allowed: %s)",
				stepIndex+1, step.Uses, key, sortedInputNames(def))
		}
	}
	for name, input := range def.Inputs {
		if !input.Required {
			continue
		}
		raw, ok := step.With[name]
		if !ok {
			return fmt.Errorf("pipeline step %d (%s): required input %q is missing", stepIndex+1, step.Uses, name)
		}
		var s string
		switch v := raw.(type) {
		case string:
			s = v
		case bool:
			s = strconv.FormatBool(v)
		default:
			s = fmt.Sprint(v)
		}
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("pipeline step %d (%s): required input %q must not be empty", stepIndex+1, step.Uses, name)
		}
	}
	return nil
}

func sortedInputNames(def *PipelineDef) string {
	names := make([]string, 0, len(def.Inputs))
	for k := range def.Inputs {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// resolveInputs returns the full substitution map (package/targets/context + inputs) with recursive substitution applied.
// Uses SubstitutionMap.MutateWith (melange-style) so input values can reference ${{package.name}} etc.
func resolveInputs(def *PipelineDef, with map[string]interface{}, s *spec.Spec) (map[string]string, error) {
	sm, err := NewSubstitutionMap(s)
	if err != nil {
		return nil, err
	}
	withMap := make(map[string]string)
	for k, v := range def.Inputs {
		withMap[k] = Substitute(v.Default, sm.Substitutions)
	}
	for k, v := range with {
		var val string
		switch x := v.(type) {
		case string:
			val = x
		case int:
			val = strconv.Itoa(x)
		case bool:
			val = strconv.FormatBool(x)
		default:
			val = fmt.Sprint(x)
		}
		withMap[k] = Substitute(val, sm.Substitutions)
	}
	return sm.MutateWith(withMap)
}

// reInputPlaceholder matches any ${{inputs.xxx}} left after known substitution (avoids bad shell substitution).
var reInputPlaceholder = regexp.MustCompile(`\$\{\{inputs\.[^}]+\}\}`)

// substituteScript replaces all Melange-style variables in script using the full substitution map.
func substituteScript(script string, inputs map[string]string) string {
	script = Substitute(script, inputs)
	// Replace any remaining ${{inputs.xxx}} with empty string so shell never sees ${{ (bad substitution)
	script = reInputPlaceholder.ReplaceAllString(script, "")
	return script
}

// buildPipelineScript turns spec.Pipeline into a single shell script.
func buildPipelineScript(s *spec.Spec) (string, error) {
	if len(s.Pipeline) == 0 {
		return "", errors.New("pipeline is required and must not be empty")
	}
	var b strings.Builder
	b.WriteString("set -e\nmkdir -p /pkg\n")
	for i, step := range s.Pipeline {
		hasRun := strings.TrimSpace(step.Run) != ""
		hasUses := step.Uses != ""
		if hasRun && hasUses {
			return "", fmt.Errorf("pipeline step %d: cannot set both 'uses' and 'run'", i+1)
		}
		if !hasRun && !hasUses {
			return "", fmt.Errorf("pipeline step %d: must set either 'uses' or 'run'", i+1)
		}
		if hasRun {
			b.WriteString(step.Run)
			if !strings.HasSuffix(strings.TrimRight(step.Run, " \t"), "\n") {
				b.WriteString("\n")
			}
			continue
		}
		def, err := getPipeline(step.Uses)
		if err != nil {
			return "", fmt.Errorf("pipeline step %d: %w", i+1, err)
		}
		if err := validatePipelineStep(def, &step, i); err != nil {
			return "", err
		}
		inputs, err := resolveInputs(def, step.With, s)
		if err != nil {
			return "", err
		}
		slog.Info("pipeline step config", "step", i+1, "uses", step.Uses, "config", inputs)
		resolved := substituteScript(def.Runs, inputs)
		b.WriteString(resolved)
		if !strings.HasSuffix(strings.TrimRight(resolved, " \t"), "\n") {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// BuildAPK produces an llb.State that contains built .apk package(s).
// It uses an Alpine-based environment: installs build deps, runs the pipeline, then creates the .apk via tar (control + data segments).
func BuildAPK(ctx context.Context, s *spec.Spec, sourceState llb.State, resolver llb.ImageMetaResolver, opts ...llb.ConstraintsOpt) (llb.State, error) {
	if s.Name == "" {
		return llb.Scratch(), errors.New("spec name is required")
	}
	if s.Version == "" {
		return llb.Scratch(), errors.New("spec version is required")
	}
	if s.Description == "" {
		return llb.Scratch(), errors.New("spec description is required")
	}
	if s.URL == "" {
		return llb.Scratch(), errors.New("spec url is required")
	}
	if s.License == "" {
		return llb.Scratch(), errors.New("spec license is required")
	}

	// Worker: Alpine + environment packages from spec (repositories + packages) + pipeline needs (deduplicated)
	workerImage := llb.Image(alpineImage, llb.WithCustomName("apk worker base"))
	if resolver != nil {
		workerImage = llb.Image(alpineImage, llb.WithMetaResolver(resolver), llb.WithCustomName("apk worker base"))
	}
	installCmd, err := buildInstallCommand(s)
	if err != nil {
		return llb.Scratch(), err
	}
	workerRunOpts := []llb.RunOption{
		llb.Args([]string{"sh", "-c", installCmd}),
		llb.WithCustomName("install build deps"),
	}
	for _, o := range opts {
		workerRunOpts = append(workerRunOpts, o)
	}
	worker := workerImage.Run(workerRunOpts...).Root()

	// Mount source at /src
	workerWithSrc := worker.File(
		llb.Copy(sourceState, "/", "/src"),
		opts...,
	)

	script, err := buildPipelineScript(s)
	if err != nil {
		return llb.Scratch(), err
	}

	// Run pipeline; output in /pkg (directory in root so it's part of state)
	pipelineRunOpts := []llb.RunOption{
		llb.Args([]string{"sh", "-c", script}),
		llb.Dir("/"),
		llb.WithCustomName("run build steps"),
	}
	for _, o := range opts {
		pipelineRunOpts = append(pipelineRunOpts, o)
	}
	builtRun := workerWithSrc.Run(pipelineRunOpts...)
	built := builtRun.Root()

	// Assembly is done in Go outside the container (see frontend: solve → export ref → AssembleAPK → solve write-apk).
	// Return only the built directory state.
	return built, nil
}
