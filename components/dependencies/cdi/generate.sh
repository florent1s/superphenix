#!/bin/bash

export VERSION="v1.63.1"

cd templates

rm *
wget https://github.com/kubevirt/containerized-data-importer/releases/download/$VERSION/cdi-operator.yaml
wget https://github.com/kubevirt/containerized-data-importer/releases/download/$VERSION/cdi-cr.yaml

sed -i "s/namespace: cdi/namespace: cdi-system/g" *
