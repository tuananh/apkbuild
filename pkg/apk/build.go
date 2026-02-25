package apk

import (
	"context"
	"fmt"
	"strings"

	"github.com/tuananh/apkbuild/pkg/spec"
	"github.com/moby/buildkit/client/llb"
	"github.com/pkg/errors"
)

const alpineImage = "alpine:3.23"

// BuildAPK produces an llb.State that contains built .apk package(s).
// It uses an Alpine-based environment: installs build deps, runs cmake, then abuild.
func BuildAPK(ctx context.Context, s *spec.Spec, sourceState llb.State, resolver llb.ImageMetaResolver, opts ...llb.ConstraintsOpt) (llb.State, error) {
	if s.Name == "" {
		return llb.Scratch(), errors.New("spec name is required")
	}
	if s.Version == "" {
		return llb.Scratch(), errors.New("spec version is required")
	}

	installDir := s.Build.InstallDir
	if installDir == "" {
		installDir = "/usr"
	}

	// Worker: Alpine + build tools (alpine-sdk, cmake, make, g++)
	workerImage := llb.Image(alpineImage, llb.WithCustomName("apk worker base"))
	if resolver != nil {
		workerImage = llb.Image(alpineImage, llb.WithMetaResolver(resolver), llb.WithCustomName("apk worker base"))
	}
	worker := workerImage.
		Run(
			llb.Args([]string{
				"sh", "-c",
				"apk add --no-cache doas alpine-sdk cmake make g++",
			}),
			llb.WithCustomName("install build deps"),
		).Root()

	// Mount source at /src
	workerWithSrc := worker.File(
		llb.Copy(sourceState, "/", "/src"),
		opts...,
	)

	// Default cmake build steps if none specified
	srcDir := "/src"
	if s.Build.SourceDir != "" && s.Build.SourceDir != "." {
		srcDir = "/src/" + s.Build.SourceDir
	}
	steps := s.Build.Steps
	if len(steps) == 0 {
		steps = []string{
			"mkdir -p /build && cd /build",
			"cmake -DCMAKE_INSTALL_PREFIX=" + installDir + " " + srcDir,
			"make -j$(nproc)",
			"make install DESTDIR=/pkg",
		}
	}

	script := "set -e\nmkdir -p /pkg\n"
	for _, s := range steps {
		script += s + "\n"
	}

	// Run cmake build; output in /pkg (directory in root so it's part of state)
	builtRun := workerWithSrc.Run(
		llb.Args([]string{"sh", "-c", script}),
		llb.Dir("/"),
		llb.WithCustomName("cmake build"),
	)
	built := builtRun.Root()

	// APKBUILD: package() copies from /input (we will mount built tree there)
	pkgname := strings.ToLower(s.Name)
	pkgver := s.Version
	pkgrel := s.Epoch
	if pkgrel == "" {
		pkgrel = "0"
	}
	pkgdesc := s.Description
	if pkgdesc == "" {
		pkgdesc = s.Name
	}
	pkgurl := s.URL
	if pkgurl == "" {
		pkgurl = "https://example.com"
	}
	pkglicense := s.License
	if pkglicense == "" {
		pkglicense = "MIT"
	}

	apkbuild := fmt.Sprintf(`# Contributor: cmake-apk
pkgname="%s"
pkgver="%s"
pkgrel="%s"
pkgdesc="%s"
url="%s"
arch="all"
license="%s"
options="!check !strip"
source=""

build() {
	true
}

package() {
	mkdir -p "$pkgdir"
	cp -a /input/pkg/* "$pkgdir"/ 2>/dev/null || cp -a /input/* "$pkgdir"/
}
`, pkgname, pkgver, pkgrel, pkgdesc, pkgurl, pkglicense)

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
