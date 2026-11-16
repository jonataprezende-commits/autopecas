#!/bin/bash
# Prepara o toolchain Go dentro do sandbox (a pasta .cache nao entra no snapshot,
# entao este script e idempotente e roda em ~35s quando precisa baixar de novo).
export PATH=/home/user/.cache/goroot/bin:$PATH
export GOPATH=/home/user/.cache/gopath
export GOMODCACHE=/home/user/.cache/gopath/pkg/mod
export GOCACHE=/home/user/.cache/gocache
export GOTOOLCHAIN=local
if [ ! -x /home/user/.cache/goroot/bin/go ]; then
  mkdir -p /home/user/.cache
  curl -sL -o /tmp/go.tgz https://go.dev/dl/go1.24.6.linux-amd64.tar.gz || { echo "FALHA no download do Go"; exit 1; }
  tar -C /home/user/.cache -xzf /tmp/go.tgz && mv /home/user/.cache/go /home/user/.cache/goroot
fi
(cd /home/user/autopecas && go mod download >/dev/null 2>&1)
go version
