#!/bin/bash

cd templates

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotclasses.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshotcontents.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/groupsnapshot.storage.k8s.io_volumegroupsnapshots.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml

wget raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml

wget https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/refs/heads/master/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
