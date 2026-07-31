#!/bin/bash

BACKUP=$1
NAMESPACE=$2
FORCE=$3

if [ "$BACKUP" = "" ]; then echo "Must provide backup name and namespace"; exit 1; fi

function kubectlgetall {
  BACKUP=$1
  NAMESPACE=$2

  for i in $(kubectl api-resources --verbs=list -o name | grep -v "events.events.k8s.io" | grep -v "events" | sort | uniq); do
    if [[ "$NAMESPACE" != "" ]]
    then
        RES=$(kubectl -n ${NAMESPACE} get --ignore-not-found ${i} -o json -l velero.io/backup-name=$BACKUP)
    else
        RES=$(kubectl get --ignore-not-found ${i} -o json -l velero.io/backup-name=$BACKUP)
    fi

    PARSED=$(echo $RES | jq --arg kind "${i}" '.items[].kind = $kind' | jq '.items | map([.kind, .apiVersion, .metadata.name, .metadata.namespace])' | jq '.[] | @sh')

    if [[ $PARSED != "" ]]; then echo -e "$PARSED \n"; fi
  done
}

echo "Cleaning up from restore of backup $BACKUP in namespace $NAMESPACE"

ALL_NS="$(kubectlgetall $BACKUP $NAMESPACE)"
ALL_NONS="$(kubectlgetall $BACKUP)"

for i in {1..2}; do
if [ $i = 1 ]; then
        echo "Doing namespaced"
	LINES="$ALL_NS"
else
	echo "Doing non-namespaced"
	LINES="$ALL_NONS"
fi

echo "$LINES" | while read i; do
  LINE=$(echo $i | tr -d '"'| tr -d "'")

  if [[ $LINE = "y" ]]; then exit 0; fi

  KIND=$(echo $LINE | cut -d ' ' -f1)
  APIVERSION=$(echo $LINE | cut -d ' ' -f2)
  NAME=$(echo $LINE | cut -d ' ' -f3)
  NAMESPACE=$(echo $LINE | cut -d ' ' -f4)

  if [[ $NAME != "" ]]; then
       echo "[!] Deletion of $APIVERSION/$KIND $NAMESPACE/$NAME"

       if [[ $FORCE = "--force" ]]; then
           echo "[!] Deletion is set to force"
       else
           read -p "[?] Type 'yes' to proceed: " USER < /dev/tty
           if [[ "$USER" != "yes" ]]; then echo "Aborting..."; exit 1; fi
       fi

       if [[ $NAMESPACE = "null" ]]; then
           kubectl delete $KIND $NAME --wait=false
       else
           kubectl delete $KIND -n $NAMESPACE $NAME --wait=false
       fi
  fi
done

done
