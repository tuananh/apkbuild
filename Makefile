IMAGE ?= tuananh/apkbuild

.PHONY: frontend example all build clean

frontend:
	docker build -t $(IMAGE) -f Dockerfile .

example:
	cd example && docker buildx build \
		--no-cache \
		-f spec.yml \
		--build-arg BUILDKIT_SYNTAX=$(IMAGE) \
		--output type=local,dest=./out \
		--progress=plain \
		.

all: example

build:
	go build -o bin/apkbuild ./cmd/frontend

clean:
	rm -rf bin/apkbuild example/out
