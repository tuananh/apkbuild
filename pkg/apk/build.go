package apk

import (
	"context"
	"fmt"
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

	if len(s.Build.Steps) == 0 {
		return llb.Scratch(), errors.New("build.steps is required and must not be empty")
	}

	script := "set -e\nmkdir -p /pkg\n"
	for _, step := range s.Build.Steps {
		script += step + "\n"
	}

	// Run build steps; output in /pkg (directory in root so it's part of state)
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
