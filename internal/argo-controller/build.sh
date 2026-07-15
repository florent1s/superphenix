#!/bin/bash
set -x

go mod tidy

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w -s' -o bin/main cmd/main.go

status=$?
if test $status -eq 0
then
    echo "build successful"
else
    echo "cancelled"
fi