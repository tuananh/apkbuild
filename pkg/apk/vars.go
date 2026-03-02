package apk

import (
	"fmt"
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
	SubstitutionTargetsContextdir   = "${{targets.contextdir}}"
	SubstitutionContextName        = "${{context.name}}"
)

// Install destination and source directory used during the build.
const (
	TargetsOutdir    = "/pkg"
	TargetsDestdir   = "/pkg"
	TargetsContextdir = "/pkg"
	PackageSrcdir    = "/src"
)

// NewSubstitutionMap returns a map of variable name -> value for the given spec.
// Used to substitute ${{package.xxx}}, ${{targets.xxx}}, ${{context.name}} in pipeline runs.
// Callers should add ${{inputs.<name>}} entries and then call Substitute on script/values.
func NewSubstitutionMap(s *spec.Spec) map[string]string {
	fullVersion := s.Version
	if s.Epoch > 0 {
		fullVersion = fmt.Sprintf("%s-r%d", s.Version, s.Epoch)
	}
	srcdir := PackageSrcdir
	if s.Build.SourceDir != "" {
		srcdir = strings.TrimSuffix(PackageSrcdir+"/"+strings.TrimPrefix(s.Build.SourceDir, "/"), "/")
	}
	return map[string]string{
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
