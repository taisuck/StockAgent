#!/bin/sh
# Windows용 바이너리를 output/stockagent.exe 로 빌드한다.
set -eu
cd "$(dirname "$0")/.."
mkdir -p output
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o output/stockagent.exe ./src
echo "built: output/stockagent.exe"
