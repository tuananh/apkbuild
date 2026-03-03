package apk

import (
	"fmt"
	"maps"
	"strings"

	"github.com/tuananh/apkbuild/pkg/spec"
)

// Substitution variable names (Melange-style). Use these in pipeline runs and in step with: values.
// See: https://github.com/chainguard-dev/melange/blob/main/pkg/config/vars.go
const (
	SubstitutionPackageName        = "${{package.name}}"
	SubstitutionPackageVersion     = "${{package.version}}"
	SubstitutionPackageFullVersion = "${{package.full-version}}"
	SubstitutionPackageEpoch       = "${{package.epoch}}"
	SubstitutionPackageDescription = "${{package.description}}"
	SubstitutionPackageSrcdir      = "${{package.srcdir}}"
	SubstitutionTargetsOutdir      = "${{targets.outdir}}"
	SubstitutionTargetsDestdir     = "${{targets.destdir}}"
	SubstitutionTargetsContextdir  = "${{targets.contextdir}}"
	SubstitutionContextName        = "${{context.name}}"
)

// Install destination and source directory used during the build.
const (
	TargetsOutdir     = "/workspace/build-out"
	TargetsDestdir    = "/workspace/build-out"
	TargetsContextdir = "/workspace/build-out"
	PackageSrcdir     = "/workspace/build-src"
)

// SubstitutionMap holds variable name -> value for pipeline substitution (melange-style).
// See: https://github.com/chainguard-dev/melange/blob/main/pkg/build/pipeline.go
type SubstitutionMap struct {
	Substitutions map[string]string
}

// NewSubstitutionMap returns a SubstitutionMap for the given spec (melange-style behavior).
// Used to substitute ${{package.xxx}}, ${{targets.xxx}}, ${{context.name}} in pipeline runs.
func NewSubstitutionMap(s *spec.Spec) (*SubstitutionMap, error) {
	fullVersion := s.Version
	if s.Epoch > 0 {
		fullVersion = fmt.Sprintf("%s-r%d", s.Version, s.Epoch)
	}
	srcdir := PackageSrcdir
	if s.Build.SourceDir != "" {
		srcdir = strings.TrimSuffix(PackageSrcdir+"/"+strings.TrimPrefix(s.Build.SourceDir, "/"), "/")
	}
	nw := map[string]string{
		SubstitutionPackageName:        s.Name,
		SubstitutionPackageVersion:     s.Version,
		SubstitutionPackageFullVersion: fullVersion,
		SubstitutionPackageEpoch:       fmt.Sprintf("%d", s.Epoch),
		SubstitutionPackageDescription: s.Description,
		SubstitutionPackageSrcdir:      srcdir,
		SubstitutionTargetsOutdir:      TargetsOutdir,
		SubstitutionTargetsDestdir:     TargetsDestdir,
		SubstitutionTargetsContextdir:  TargetsContextdir,
		SubstitutionContextName:        s.Name,
	}
	return &SubstitutionMap{Substitutions: nw}, nil
}

// MutateWith merges "with" into a clone of the substitution map (as ${{inputs.<key>}}),
// then performs recursive substitution on all values so they can reference each other.
// Returns the resulting map. Mirrors melange's SubstitutionMap.MutateWith.
func (sm *SubstitutionMap) MutateWith(with map[string]string) (map[string]string, error) {
	nw := maps.Clone(sm.Substitutions)
	for k, v := range with {
		if strings.HasPrefix(k, "${{") {
			nw[k] = v
		} else {
			nw["${{inputs."+k+"}}"] = v
		}
	}
	// Recursive substitution until fixed point (melange MutateStringFromMap behavior).
	for i := 0; i < 32; i++ {
		changed := false
		for k, v := range nw {
			replaced := Substitute(v, nw)
			if replaced != v {
				changed = true
				nw[k] = replaced
			}
		}
		if !changed {
			break
		}
	}
	return nw, nil
}

// Substitute replaces all ${{...}} variables in s with values from m.
// Keys in m must include the ${{...}} form (e.g. "${{package.name}}").
// Any remaining ${{...}} after substitution are left as-is (caller can strip them if needed).
func Substitute(s string, m map[string]string) string {
	out := s
	for k, v := range m {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}
