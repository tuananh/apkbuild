package frontend

import (
	"context"

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

	// Build APK: produces state with .apk in /out (nil resolver: worker resolves images during solve)
	st, err := apk.BuildAPK(ctx, spec, *bctx, nil)
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "marshal llb")
	}

	res, err := client.Solve(ctx, gwclient.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}
