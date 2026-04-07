#!/bin/bash

REPO=$1

# If no repo is specified, we're not cloning anything
if [ -z "$REPO" ]; then exit 0; fi

export GIT_SSH_COMMAND="ssh -i /id_rsa -o IdentitiesOnly=yes -o StrictHostKeyChecking=no"
git clone -b main $REPO remote 2>&1