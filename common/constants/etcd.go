package constants

const EtcdPreStopCommand  = `certConfig="--ca-file=/etc/cert/ca.crt --cert-file=/etc/cert/peer.crt --key-file=/etc/cert/peer.key"
etcdctl --endpoints="https://127.0.0.1:2379" ${certConfig} member remove $(etcdctl --endpoints="https://127.0.0.1:2379" ${certConfig} member list | grep $(hostname) | cut -d':' -f1)
rm -rf /var/lib/etcd`

const EtcdStartCommand = `HOSTNAME=$(hostname)
index=${HOSTNAME##*-}

CLUSTER_NAME=$(echo "$HOSTNAME" | head -c-3)
nodeDomain=https://${HOSTNAME}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
initialAdvertisePeerUrls=${nodeDomain}:2380
advertiseClientUrls=${nodeDomain}:2379
dataDir=/var/lib/etcd
initialCluster=""
initialClusterState="existing"

EchoLog() {
logTime=$(date "+%Y-%m-%d %H:%M:%S")
echo "[ ${logTime} ] $1"
}

RemoveMember() {
memberId=""
for i in $(seq 5); do
  memberId=$(etcdctl --endpoints="https://$2:2379" $1 member list 2>&1 | grep $(hostname) | awk '{print $1}' | sed 's/://g')
  if [ "${memberId}" != "" ]; then break;fi
done
if [ "${memberId}" != "" ]; then
  echo "This member ${HOSTNAME} and member id is ${memberId} already exists in the cluster and will be removed ..."
  for i in $(seq 5); do
	removeResult=$(etcdctl --endpoints="https://$2:2379" $1 member remove ${memberId} 2>&1 || true)
	if echo "${removeResult}" | grep "Removed member" &>/dev/null; then echo "${removeResult}"; break; fi
  done
fi
}

echo "Init node ${HOSTNAME} ..."

echo "-----------------------------------------------------------------------------"
nodeName=${CLUSTER_NAME}-${index}
domain=${nodeName}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
EchoLog "Waiting for ${domain} to come up"
while ! ping -W 1 -c 1 ${domain} >/dev/null; do sleep 3; done
sleep 8
echo "-----------------------------------------------------------------------------"

certConfig="--ca-file=/etc/cert/ca.crt --cert-file=/etc/cert/peer.crt --key-file=/etc/cert/peer.key"
CLUSTER_ENDPOINT=${CLUSTER_NAME}-client.${CLUSTER_NAMESPACE}

# Determine if the cluster already exists
EchoLog "Determine if the cluster already exists ..."
checkEndpoint=1
addEndpoints=""
for i in $(seq 5); do
listResult=$(etcdctl --endpoints="https://${CLUSTER_ENDPOINT}:2379" ${certConfig} cluster-health 2>&1 || true)
healthyCount=$(echo "${listResult}" | grep "is healthy:" | wc -l)
if [ "${healthyCount}" != "0" ]; then
  addEndpoints=$(echo -e "${listResult}" | head -n1 | awk '{print $9}')
  checkEndpoint=0
  break
fi
done

if [ "${addEndpoints}" == "" ]; then
  addEndpoints="https://${CLUSTER_ENDPOINT}:2379"
else
  RemoveMember "${certConfig}" "${CLUSTER_ENDPOINT}"
fi

if [ "${index}" -eq "0" ]; then
  if [ "${checkEndpoint}" -ne "0" ]; then
    initialCluster="${HOSTNAME}=${nodeDomain}:2380"
    initialClusterState=new
  else
    EchoLog "etcdctl --endpoints=\"${addEndpoints}\" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380"
    addResult=$(etcdctl --endpoints="${addEndpoints}" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380)
    echo "${addResult}"
    initialCluster=$(echo "${addResult}" | grep ETCD_INITIAL_CLUSTER= | sed 's/ETCD_INITIAL_CLUSTER=//g;s/"//g')
    echo "initialCluster: $initialCluster"
  fi
else
  echo ""; EchoLog "Waiting for the previous nodes become ready,sleeping..."
  for i in $(seq 0 $((${index} - 1))); do
    nodeName=${CLUSTER_NAME}-${i}
    domain=${nodeName}.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}
    echo "etcdctl --endpoints=\"https://${CLUSTER_ENDPOINT}:2379\" ${certConfig} cluster-health 2>&1 | grep ${domain} | grep \"is health\""
    while true; do
	  etcdctl --endpoints="https://${CLUSTER_ENDPOINT}:2379" ${certConfig} cluster-health 2>&1 | grep ${domain} | grep "is health" && break
	  sleep 3
    done
  done
  addEndpoints="https://${CLUSTER_NAME}-0.${CLUSTER_NAME}.${CLUSTER_NAMESPACE}"
  echo ""; EchoLog "Check if the etcd cluster is healthy ..."
  while true; do
    healthCheck=$(etcdctl --endpoints="${addEndpoints}:2379" ${certConfig} cluster-health 2>&1 | tail -n1)
    if [ "${healthCheck}" == "cluster is healthy" ]; then
	  sleep 3; break
    fi
  done
  echo ""; EchoLog "The etcd cluster is healthy, next add member ..."
  echo "etcdctl --endpoints=\"${addEndpoints}:2379\" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380"
  addResult=""
  while true; do
    addResult=$(etcdctl --endpoints="${addEndpoints}:2379" ${certConfig} member add ${HOSTNAME} ${nodeDomain}:2380 || true)
    if [ "$?" == "0" ]; then break; fi
  done
  echo "${addResult}"
  EchoLog "The member ${HOSTNAME} has been added to cluster"
  initialCluster=$(echo "${addResult}" | grep ETCD_INITIAL_CLUSTER= | sed 's/ETCD_INITIAL_CLUSTER=//g;s/"//g')
  echo "initialCluster: $initialCluster"
fi

rm -rf ${dataDir}/*
mkdir -p ${dataDir}

echo ""
echo "=============================================================================="

echo ""
echo "etcd --name=${HOSTNAME} \\"
echo "--initial-advertise-peer-urls=${initialAdvertisePeerUrls} \\"
echo "--listen-peer-urls=https://0.0.0.0:2380 \\"
echo "--listen-client-urls=https://0.0.0.0:2379 \\"
echo "--advertise-client-urls=${advertiseClientUrls} \\"
echo "--initial-cluster-token=${CLUSTER_NAME} \\"
echo "--data-dir=${dataDir} \\"
echo "--initial-cluster=${initialCluster} \\"
echo "--initial-cluster-state=${initialClusterState} \\"
echo "--client-cert-auth=true \\"
echo "--peer-client-cert-auth=true \\"
echo "--peer-cert-file=/etc/cert/peer.crt \\"
echo "--peer-key-file=/etc/cert/peer.key \\"
echo "--peer-trusted-ca-file=/etc/cert/ca.crt \\"
echo "--trusted-ca-file=/etc/cert/ca.crt \\"
echo "--cert-file=/etc/cert/server.crt \\"
echo "--key-file=/etc/cert/server.key"
echo ""
echo "=============================================================================="

echo ""
exec etcd --name=${HOSTNAME} \
  --initial-advertise-peer-urls=${initialAdvertisePeerUrls} \
  --listen-peer-urls=https://0.0.0.0:2380 \
  --listen-client-urls=https://0.0.0.0:2379 \
  --advertise-client-urls=${advertiseClientUrls} \
  --initial-cluster-token=${CLUSTER_NAME} \
  --data-dir=${dataDir} \
  --initial-cluster=${initialCluster} \
  --initial-cluster-state=${initialClusterState} \
  --client-cert-auth=true \
  --peer-client-cert-auth=true \
  --peer-cert-file=/etc/cert/peer.crt \
  --peer-key-file=/etc/cert/peer.key \
  --peer-trusted-ca-file=/etc/cert/ca.crt \
  --trusted-ca-file=/etc/cert/ca.crt \
  --cert-file=/etc/cert/server.crt \
  --key-file=/etc/cert/server.key`
