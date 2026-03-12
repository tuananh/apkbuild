package frontend

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerui"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/pkg/errors"
	"github.com/tuananh/apkbuild/pkg/apk"
)

// BuildFunc is the BuildKit gateway BuildFunc that reads the YAML spec from the
// build context (Dockerfile) and produces an APK package.
func BuildFunc(ctx context.Context, client gwclient.Client) (*gwclient.Result, error) {
	dc, err := dockerui.NewClient(client)
	if err != nil {
		return nil, errors.Wrap(err, "dockerui client")
	}

	// Read spec: Dockerfile input is our YAML spec (e.g. docker build -f spec.yml .)
	src, err := dc.ReadEntrypoint(ctx, "Dockerfile")
	if err != nil {
		return nil, errors.Wrap(err, "read spec file (use -f spec.yml)")
	}

	spec, err := LoadSpec(src.Data)
	if err != nil {
		return nil, errors.Wrap(err, "parse spec yaml")
	}

	// Build context = main context (sources)
	bctx, err := dc.MainContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "build context")
	}

	// Forward --no-cache from buildx so BuildKit ignores cache for all steps
	var buildOpts []llb.ConstraintsOpt
	if dc.IsNoCache("") {
		buildOpts = append(buildOpts, llb.IgnoreCache)
	}

	// Build APK: produces state with built directory only (assembly is done in Go below)
	st, err := apk.BuildAPK(ctx, spec, *bctx, nil, buildOpts...)
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "marshal llb")
	}

	// Solve: get ref to built directory
	res, err := client.Solve(ctx, gwclient.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, err
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	// Copy ref at apk.TargetsOutdir to a temp dir so we can run AssembleAPK in Go
	tmpDir, err := os.MkdirTemp("", "apkbuild-ref-")
	if err != nil {
		return nil, errors.Wrap(err, "mk temp dir")
	}
	defer os.RemoveAll(tmpDir)

	if err := copyRefToDir(ctx, ref, apk.TargetsOutdir, tmpDir); err != nil {
		return nil, errors.Wrap(err, "copy build-out from ref")
	}

	// Use nested build-out if present (same as previous shell behavior)
	dataDir := tmpDir
	if info, err := os.Stat(filepath.Join(tmpDir, "build-out")); err == nil && info.IsDir() {
		dataDir = filepath.Join(tmpDir, "build-out")
	}

	apkPath := filepath.Join(tmpDir, "out.apk")
	if err := apk.AssembleAPK(dataDir, apkPath, spec); err != nil {
		return nil, errors.Wrap(err, "assemble apk")
	}

	apkBytes, err := os.ReadFile(apkPath)
	if err != nil {
		return nil, errors.Wrap(err, "read apk file")
	}

	apkName := fmt.Sprintf("%s-%s-r%d.apk", strings.ToLower(spec.Name), spec.Version, spec.Epoch)
	b64 := base64.StdEncoding.EncodeToString(apkBytes)

	// Second solve: run in an image that has sh+base64 (scratch has no shell), then copy apk to scratch
	const writeAPKImage = "alpine:3.23"
	written := llb.Image(writeAPKImage).Run(
		llb.Args([]string{"sh", "-c", "mkdir -p /out && echo \"$APK_B64\" | base64 -d > \"/out/$APK_NAME\""}),
		llb.AddEnv("APK_B64", b64),
		llb.AddEnv("APK_NAME", apkName),
		llb.WithCustomName("write apk"),
	).Root()
	writeAPK := llb.Scratch().File(llb.Copy(written, "/out/"+apkName, "/"))

	def2, err := writeAPK.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "marshal write-apk llb")
	}

	res2, err := client.Solve(ctx, gwclient.SolveRequest{
		Definition: def2.ToPB(),
	})
	if err != nil {
		return nil, err
	}

	return res2, nil
}

// copyRefToDir recursively copies the ref at refPath into local dir destDir.
func copyRefToDir(ctx context.Context, ref gwclient.Reference, refPath, destDir string) error {
	entries, err := ref.ReadDir(ctx, gwclient.ReadDirRequest{Path: refPath})
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Path
		if name == "" || name == "." || name == ".." {
			continue
		}
		srcPath := refPath + "/" + name
		dstPath := filepath.Join(destDir, filepath.FromSlash(name))
		if e.Mode&uint32(os.ModeDir) != 0 {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyRefToDir(ctx, ref, srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := ref.ReadFile(ctx, gwclient.ReadRequest{Filename: srcPath})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		// Preserve mode from ref (e.g. 0755 for binaries from make install)
		mode := e.Mode & uint32(os.ModePerm)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dstPath, data, os.FileMode(mode)); err != nil {
			return err
		}
	}
	return nil
}
