package main

import (
	"os"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/tuananh/apkbuild/frontend"
)

func main() {
	ctx := appcontext.Context()
	if err := grpcclient.RunFromEnvironment(ctx, frontend.BuildFunc); err != nil {
		os.Exit(1)
	}
}
