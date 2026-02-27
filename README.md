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
- **YAML spec**: name, version, epoch, url, license, description, sources (build context), and optional build steps / install dir / source subdir.
- **Build backend**: Alpine image + `alpine-sdk`, `cmake`, `make`, `g++` → CMake build → `abuild` → `.apk` files.
- **Output**: One or more `.apk` files at the result root (e.g. with `--output type=local,dest=./out`).

## Build the frontend image

```bash
docker build -t tuananh/apkbuild -f Dockerfile .
```

Our demo syntax would look like this

```yaml
name: hello
version: "1.0.0"
epoch: 0
url: https://github.com/tuananh/apkbuild
license: MIT
description: Minimal package demo

dependencies:
  runtime: []

environment:
  contents:
    repositories:
      - https://dl-cdn.alpinelinux.org/alpine/edge/main
    packages:
      - alpine-baselayout-data
      - busybox
      - build-base
      - scanelf
      - ssl_client
      - ca-certificates-bundle
      - cmake
      - make
      - g++
      - alpine-sdk
      - doas

sources:
  app:
    context: {}

build:
  source_dir: hello  # project lives in hello/ relative to build context
  steps:
    - mkdir -p /build && cd /build
    - cmake -DCMAKE_INSTALL_PREFIX=/usr /src/hello
    - make -j$(nproc)
    - make install DESTDIR=/pkg

```

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

`BUILDKIT_SYNTAX` is the frontend syntax you built eawrlier.

The `example/` directory contains:

- `spec.yml` — spec with name, version, epoch, url, license, and `source_dir: hello`
- `hello/` — minimal CMake C++ project (`CMakeLists.txt`, `main.cpp`). For demonstration purpose, I don't want to implement `git clone`

After a successful build, `./out` contains the generated `.apk` file(s).

## Layout

- **`cmd/frontend/`** — Gateway entrypoint (runs the BuildKit frontend).
- **`frontend/`** — Custom frontend: spec loading and gateway `BuildFunc` (reads YAML, gets context, calls APK build).
- **`pkg/spec/`** — YAML spec struct and `Load()`.
- **`pkg/apk/`** — Build backend: LLB for Alpine + cmake + abuild and `.apk` output.
- **`example/`** — Sample spec and `hello` CMake project.

## Requirements

- Docker with BuildKit (e.g. `DOCKER_BUILDKIT=1` or Docker 23+).
- For a remote frontend image: push `tuananh/apkbuild` to a registry and use that ref in `BUILDKIT_SYNTAX`.
