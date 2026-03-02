# Predefined pipeline scripts

Each YAML file defines one pipeline that can be referenced with `uses: <name>` in a spec’s `pipeline`. The file path under `pipelines/` (without `.yaml`) is the use name (e.g. `autoconf/configure.yaml` → `uses: autoconf/configure`).

**Schema:**

- `name` (optional): Human-readable description.
- `needs` (optional): Dependencies required in the build environment:
  - **`needs.packages`**: List of Alpine package names (e.g. `wget`, `cmake`, `build-base`). The build backend collects these from every pipeline step used in a spec, deduplicates them, merges with `environment.contents.packages`, and installs all of them before running the pipeline. You do not need to list these in the spec’s environment unless you want to pin versions or add repos.
- `inputs` (optional): Map of input name → schema (melange-style). The spec’s `with:` is validated against this:
  - **Short form**: `name: "default"` — optional input with default value.
  - **Long form**: `name: { description?: string, default?: string, required?: bool }` — human-readable description, optional default, or required (must be provided in `with:`).
  - Only inputs declared here are allowed in `with:`; unknown keys are rejected.
- `runs` (required): Shell script body. Supports variable substitution (Melange-style, see below).

Builds run from `/`; source/context is at `/src`, install destination is `/pkg`.

**Variable substitution** (usable in `runs` and in step `with:` values):

| Variable | Description |
|----------|-------------|
| `${{package.name}}` | Package name from spec |
| `${{package.version}}` | Package version |
| `${{package.full-version}}` | Version + epoch, e.g. `1.0.0-r0` |
| `${{package.epoch}}` | Epoch number |
| `${{package.description}}` | Package description |
| `${{package.srcdir}}` | Source directory (default `/src`, or `/src/<source_dir>` if `build.source_dir` is set) |
| `${{targets.outdir}}` | Output root (`/pkg`) |
| `${{targets.destdir}}` | Install destination (`/pkg`) |
| `${{targets.contextdir}}` | Same as destdir (`/pkg`) |
| `${{context.name}}` | Package name (same as `package.name`) |
| `${{inputs.<name>}}` | Value of pipeline input from step `with:` (or default) |
