# Predefined pipeline scripts

Each YAML file defines one pipeline that can be referenced with `uses: <name>` in a spec’s `pipeline`. The file path under `pipelines/` (without `.yaml`) is the use name (e.g. `autoconf/configure.yaml` → `uses: autoconf/configure`).

**Schema:**

- `name` (optional): Human-readable description.
- `inputs` (optional): Map of input name → schema (melange-style). The spec’s `with:` is validated against this:
  - **Short form**: `name: "default"` — optional input with default value.
  - **Long form**: `name: { description?: string, default?: string, required?: bool }` — human-readable description, optional default, or required (must be provided in `with:`).
  - Only inputs declared here are allowed in `with:`; unknown keys are rejected.
- `runs` (required): Shell script body. Supports `${{inputs.xxx}}`, `${{package.name}}`, `${{package.version}}`, `${{targets.contextdir}}` (/pkg).

Builds run from `/`; source/context is at `/src`, install destination is `/pkg`.
