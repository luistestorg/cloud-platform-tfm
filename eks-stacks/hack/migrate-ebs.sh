#!/bin/bash

#ns=( $(kubectl get pvc -A --no-headers | grep redis-data-redis-master-0 | grep -v nativelink-shared | grep -v gp3-enc | grep 3Gi | cut -d' ' -f1 - | xargs) )
ns=( "nativelink-bc7a47c89f6b7h" )
for n in "${ns[@]}"
do
  echo -e "\nMigrating Redis PVC for ${n}\n"
  kubens "${n}"

  snapName="redis-data-pvc-snap-${n}"

  # create a backup
  rm create-snapshot.yaml || true
  cat <<EOF | echo "apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: ${snapName}
spec:
  volumeSnapshotClassName: gp3-snapshotclass
  source:
    persistentVolumeClaimName: redis-data-redis-master-0
"> create-snapshot.yaml | kubectl apply -f create-snapshot.yaml -n "${n}"
EOF

  maxLoops=120
  maxWait=$(( 5*maxLoops ))
  echo -e "\nCreated new volumesnapshot ${snapName} ... will loop for up to ${maxWait} seconds to see it become ready to use.\n"

  # wait for the snapsnot to complete
  readyToUse="false"
  loops=0
  while [ "${readyToUse}" != "true" ]; do
    loops=$((loops+1))
    if [ "$loops" == "${maxLoops}" ]; then
      echo "ERROR: snapshot for redis in ${n} not ready after ${maxWait} seconds, failing ..."
      exit 1
    fi
    sleep 5
    kubectl describe volumesnapshot ${snapName} -n "${n}" >> /dev/null 2>&1
    readyToUse=$(kubectl get volumesnapshot ${snapName} -n "${n}" -o jsonpath={.status.readyToUse} | xargs)
    waited=$(( 5*loops ))
    echo "readyToUse? ${readyToUse}, after waiting $waited secs so far ..."
  done
  took=$(( 5*loops ))
  echo -e "\nBackup is ready? ${readyToUse} (took $took secs), replacing the PVC\n"

  kubectl scale sts redis-master --replicas=0 -n "${n}"
  sleep 2
  kubectl patch pvc redis-data-redis-master-0 --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]' -n "${n}"
  sleep 2
  kubectl delete pvc redis-data-redis-master-0 -n "${n}"

  rm create-pvc.yaml || true
  cat <<EOF | echo "apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    volume.kubernetes.io/storage-provisioner: ebs.csi.aws.com
  labels:
    app.kubernetes.io/component: master
    app.kubernetes.io/instance: redis-${n}
    app.kubernetes.io/name: redis
  name: redis-data-redis-master-0
  namespace: ${n}
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 3Gi
  storageClassName: gp3-enc
  volumeMode: Filesystem
  dataSource:
    name: ${snapName}
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
"> create-pvc.yaml | kubectl apply -f create-pvc.yaml -n "${n}"
EOF

  echo -e "\nCreated new PVC from snapshot, bring Redis back up ...\n"
  kubectl scale sts redis-master --replicas=1 -n "${n}"

  echo -e "\nWaiting for redis-master to recover\n"
  kubectl wait --for=condition=Ready pod redis-master-0 -n "${n}" --timeout=180s
  kubectl rollout restart deploy cas

  #kubectl patch volumesnapshot ${snapName} --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]' -n "${n}"
  #sleep 5
  #kubectl delete volumesnapshot ${snapName} -n "${n}" --force

done

