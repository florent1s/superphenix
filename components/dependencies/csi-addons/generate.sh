#!/bin/bash

export RELEASE="v0.13.0"

cd templates

wget https://github.com/csi-addons/kubernetes-csi-addons/releases/download/${RELEASE}/crds.yaml
wget https://github.com/csi-addons/kubernetes-csi-addons/releases/download/${RELEASE}/rbac.yaml
wget https://github.com/csi-addons/kubernetes-csi-addons/releases/download/${RELEASE}/setup-controller.yaml
