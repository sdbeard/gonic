# ==================================================================================
# The MIT License (MIT)
#
# Copyright (c) 2024 Sean Beard
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in the
# Software without restriction, including without limitation the rights to use,
# copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the
# Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN
# AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTIO
# WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
# ==================================================================================
SHELL := /bin/bash
BINARY ?= gonic
VERSION ?= 1.0.0
#REGISTRY ?= aws.testlab.local:4510/${BINARY}
#DOCKER_REMOTE_HOST ?= "ssh://testlab"
ENV ?= dev

# Phony targets
.PHONY: deps-reset tidy deps-upgrade deps-cleancache clean debug

debug: clean
	@go build -o ./build/$(BINARY) ./cmd/gonic/*.go
	./build/$(BINARY) -music-path ./build/gonicdir/music -podcast-path ./build/gonicdir/podcast -cache-path ./build/gonicdir/cache -playlists-path ./build/gonicdir/playlist -db-path ./build/gonicdir/data/db

clean:
	$(info Cleaning previous build...)
	go clean
	@if [ -f ./build/$(BINARY) ]; then rm -rf ./build/$(BINARY); fi


# ==================================================================================
# Modules support

deps-reset:
	git checkout -- go.mod
	go mod tidy
	go mod vendor

tidy:
	go mod tidy
	go mod vendor

deps-upgrade:
	go get -u -t -d -v ./...
	go mod tidy
	go mod vendor

deps-cleancache:
	go clean -modcache

# ==================================================================================
