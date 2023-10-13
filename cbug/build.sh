#!/bin/bash

version=v1.1.0

function build-single {
	rm -rf release/cbug
	cp -a release/template release/cbug
	touch release/cbug/release-info.json
	echo {\"version\": \"$version\", \"target\": \"$target\", \"architecture\": \"$targetarch\"} > release/cbug/release-info.json
	GOARCH=$arch GOOS=$os go build -buildmode exe -o release/cbug/bin/ cbug.go
	cd release
	zip -r $version/cbug-$target-$version.zip cbug -x "*.DS_Store"
	cd ..
	rm -rf release/cbug
}

targetarch=x86
target=macos-x86
arch=amd64
os=darwin
echo "building all platform executables"
build-single
os=linux
target=linux-x86
build-single
os=windows
target=windows-x86
build-single
targetarch=arm64
arch=arm64
target=windows-arm
build-single
os=linux
target=linux-arm
build-single
os=darwin
target=macos-arm
build-single


