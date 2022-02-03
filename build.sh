#!/usr/bin/env bash

img="sgaunet/k8see-importer"
docker build . -t ${img}:latest
rc=$?

if [ "$rc" != "0" ]
then
    echo "Build FAILED. EXIT"
    exit 1
fi

docker push ${img}:latest