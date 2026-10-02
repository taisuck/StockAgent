#!/bin/sh
# Linux용 바이너리를 output/stockagent 로 빌드한다.
set -eu
cd "$(dirname "$0")/.."
mkdir -p output
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o output/stockagent ./src/stockagent
echo "built: output/stockagent"
