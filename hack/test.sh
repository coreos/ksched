#!/usr/bin/env bash

# This script should be called from 'ksched' root dir.

set -eo pipefail

go vet ./... 2>&1

SRCS=`find . -name \*.go`
fmtres=`gofmt -l -d -s $SRCS 2>&1`
if [ ! -z "$fmtres" ]; then
	echo -e "gofmt checking failed:\n$fmtres"
	exit 1
fi

# Run all tests in all packages
go test ./... --race

echo "success!"
