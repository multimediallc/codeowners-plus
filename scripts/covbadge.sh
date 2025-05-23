#!/bin/sh -l

set -e

go install github.com/AlexBeauchemin/gobadge@v0.4.0

# fail if gobadge is no installed
which gobadge

go test ./... -covermode=count -coverprofile=coverage.out
go tool cover -func=coverage.out -o=coverage.out
gobadge -filename=coverage.out
