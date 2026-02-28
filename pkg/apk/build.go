package apk

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/pkg/errors"
	"github.com/tuananh/apkbuild/pkg/spec"
)

const alpineImage = "alpine:3.23"

// buildInstallCommand returns a shell script that configures apk repos (if any) and installs packages from the spec.
func buildInstallCommand(s *spec.Spec) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, repo := range s.Environment.Contents.Repositories {
		b.WriteString(fmt.Sprintf("echo %q >> /etc/apk/repositories\n", repo))
	}
	if len(s.Environment.Contents.Packages) > 0 {
		b.WriteString("apk add --no-cache ")
		b.WriteString(strings.Join(s.Environment.Contents.Packages, " "))
		b.WriteString("\n")
	}
	return b.String()
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

// resolveInputs returns a map of input name -> string value (from step.With and pipeline defaults).
func resolveInputs(def *PipelineDef, with map[string]interface{}, s *spec.Spec) map[string]string {
	out := make(map[string]string)
	for k, v := range def.Inputs {
		out[k] = v.Default
	}
	for k, v := range with {
		key := k
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
		val = substitutePackage(val, s)
		out[key] = val
	}
	return out
}

func substitutePackage(tpl string, s *spec.Spec) string {
	tpl = strings.ReplaceAll(tpl, "${{package.name}}", s.Name)
	tpl = strings.ReplaceAll(tpl, "${{package.version}}", s.Version)
	return tpl
}

// reInputPlaceholder matches any ${{inputs.xxx}} left after known substitution (avoids bad shell substitution).
var reInputPlaceholder = regexp.MustCompile(`\$\{\{inputs\.[^}]+\}\}`)

// substituteScript replaces ${{inputs.xxx}}, ${{package.xxx}}, ${{targets.contextdir}} in script.
func substituteScript(script string, inputs map[string]string, s *spec.Spec) string {
	script = substitutePackage(script, s)
	script = strings.ReplaceAll(script, "${{targets.contextdir}}", "/pkg")
	for k, v := range inputs {
		script = strings.ReplaceAll(script, "${{inputs."+k+"}}", v)
	}
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
		inputs := resolveInputs(def, step.With, s)
		resolved := substituteScript(def.Runs, inputs, s)
		b.WriteString(resolved)
		if !strings.HasSuffix(strings.TrimRight(resolved, " \t"), "\n") {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// BuildAPK produces an llb.State that contains built .apk package(s).
// It uses an Alpine-based environment: installs build deps, runs cmake, then abuild.
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

	// Worker: Alpine + environment packages from spec (repositories + packages)
	workerImage := llb.Image(alpineImage, llb.WithCustomName("apk worker base"))
	if resolver != nil {
		workerImage = llb.Image(alpineImage, llb.WithMetaResolver(resolver), llb.WithCustomName("apk worker base"))
	}
	installCmd := buildInstallCommand(s)
	worker := workerImage.
		Run(
			llb.Args([]string{"sh", "-c", installCmd}),
			llb.WithCustomName("install build deps"),
		).Root()

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
	builtRun := workerWithSrc.Run(
		llb.Args([]string{"sh", "-c", script}),
		llb.Dir("/"),
		llb.WithCustomName("run build steps"),
	)
	built := builtRun.Root()

	// APKBUILD: package() copies from /input (we will mount built tree there)
	pkgname := strings.ToLower(s.Name)
	pkgver := s.Version
	pkgrel := fmt.Sprintf("%d", s.Epoch)
	pkgdeps := strings.Join(s.Dependencies.Runtime, " ")

	apkbuild := fmt.Sprintf(`# Contributor: cmake-apk
pkgname="%s"
pkgver="%s"
pkgrel="%s"
pkgdesc="%s"
url="%s"
arch="all"
license="%s"
depends="%s"
options="!check !strip"
source=""

build() {
	true
}

package() {
	mkdir -p "$pkgdir"
	cp -a /input/pkg/* "$pkgdir"/ 2>/dev/null || cp -a /input/* "$pkgdir"/
}
`, pkgname, pkgver, pkgrel, s.Description, s.URL, s.License, pkgdeps)

	apkbuildState := llb.Scratch().File(
		llb.Mkfile("APKBUILD", 0644, []byte(apkbuild)),
		opts...,
	)

	// /work: APKBUILD + mount built at /input for package()
	workDir := llb.Scratch().
		File(llb.Copy(apkbuildState, "APKBUILD", "APKBUILD"), opts...)

	// Create builder user and add to abuild group (abuild requires user to be in group abuild).
	abuildScript := "set -e\n" +
		"adduser -D builder\n" +
		"addgroup builder abuild\n" +
		"mkdir -p /out\n" +
		"chown -R builder:builder /work /out\n" +
		"su builder -s /bin/sh -c 'abuild-keygen -an'\n" +
		"cp /home/builder/.abuild/*.rsa.pub /etc/apk/keys/\n" +
		"su builder -s /bin/sh -c 'cd /work && abuild -r && find ~/packages -name \"*.apk\" -exec cp {} /out \\;'\n"
	abuildRun := worker.Run(
		llb.Args([]string{"sh", "-c", abuildScript}),
		llb.AddMount("/work", workDir),
		llb.AddMount("/input", built, llb.Readonly),
		llb.WithCustomName("abuild package"),
	).Root()

	result := llb.Scratch().File(
		llb.Copy(abuildRun, "/out", "/"),
		opts...,
	)
	return result, nil
}
