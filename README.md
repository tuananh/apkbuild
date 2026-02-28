# apkbuild

This project is a **demonstration of how to use BuildKit** to build anything you want.

In this example, I will use BuildKit to package software into APK (Alpine) format, something similar to Chainguard's melange but more naive.

To do that, we will need to:

1. **Define a custom frontend** — effectively a custom syntax for BuildKit. The frontend reads your input (here, a YAML spec) and produces a low-level build (LLB) definition instead of using the default Dockerfile syntax.

2. **Implement a build backend** — the logic that actually builds what you want. Here that means: CMake build plus packaging into APK (Alpine package format). The backend could do anything (other package formats, other toolchains, etc.).

For this demo, **APK packaging uses `abuild`** internally (Alpine’s standard tooling). The same idea could be implemented with something like **apk-go** or another packager; the frontend/backend split stays the same.

I'm being lazy here and just assume CMake & hard-coding the APK packaging steps but you get the idea.

---

## Overview

- **Custom frontend**: BuildKit gateway that reads a YAML spec from the build context (the “Dockerfile” input) and turns it into LLB.
- **YAML spec** (melange-style): name, version, epoch, url, license, description, **environment** (repositories + packages), top-level **pipeline** (`uses:` or `run:`), optional sources / install_dir / source_dir.
- **Build backend**: Alpine image + environment packages → pipeline (fetch / cmake or autoconf / strip) → `abuild` → `.apk` files.
- **Output**: One or more `.apk` files at the result root (e.g. with `--output type=local,dest=./out`).

## Build the frontend image

```bash
docker build -t tuananh/apkbuild -f Dockerfile .
```

Our example fetches [hello-package](https://github.com/tuananh/hello-package) and builds with CMake:

```yaml
name: hello
version: "1.0.0"
epoch: 0
url: https://github.com/tuananh/hello-package
license: MIT
description: Minimal hello package

dependencies:
  runtime: []

environment:
  contents:
    repositories:
      - https://dl-cdn.alpinelinux.org/alpine/edge/main
    packages:
      - alpine-sdk
      - ca-certificates-bundle

pipeline:
  - uses: fetch
    with:
      uri: https://github.com/tuananh/hello-package/archive/refs/heads/main.tar.gz
      expected-none: true
      strip-components: 1
  - uses: cmake/configure
  - uses: cmake/make
  - uses: cmake/make-install
  - uses: strip
```

Pipeline steps: **`uses:`** (predefined) or **`run:`** (inline script). Supported `uses`: `fetch`, `cmake/configure`, `cmake/make`, `cmake/make-install`, `autoconf/configure`, `autoconf/make`, `autoconf/make-install`, `strip`. Each pipeline declares its own **`needs.packages`** (e.g. fetch needs `wget`); the build backend collects and installs these automatically (deduplicated with `environment.contents.packages`). You only need to list extra env packages (e.g. `alpine-sdk` for abuild, `ca-certificates-bundle` for HTTPS fetch).

## Build the package

Use the frontend as the BuildKit syntax and point it at your spec and context:

```bash
cd example
docker buildx build \
  -f spec.yml \
  --build-arg BUILDKIT_SYNTAX=tuananh/apkbuild \
  --output type=local,dest=./out \
  .
```

`BUILDKIT_SYNTAX` is the frontend image you built earlier.

The `example/` directory contains:

- `spec.yml` — melange-style spec (hello-package: fetch from GitHub + cmake pipeline + strip)
- `hello/` — optional minimal CMake project (for local build context if you use `sources` instead of fetch)

After a successful build, `./out` contains the generated `.apk` file(s).

## Layout

- **`cmd/frontend/`** — Gateway entrypoint (runs the BuildKit frontend).
- **`frontend/`** — Custom frontend: spec loading and gateway `BuildFunc` (reads YAML, gets context, calls APK build).
- **`pkg/spec/`** — YAML spec struct and `Load()`.
- **`pkg/apk/`** — Build backend: LLB for Alpine + pipeline scripts + abuild and `.apk` output.
- **`example/`** — Sample spec (hello-package) and optional `hello/` CMake project.

## Requirements

- Docker with BuildKit (e.g. `DOCKER_BUILDKIT=1` or Docker 23+).
- For a remote frontend image: push `tuananh/apkbuild` to a registry and use that ref in `BUILDKIT_SYNTAX`.
