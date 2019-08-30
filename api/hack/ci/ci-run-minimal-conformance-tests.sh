#!/usr/bin/env bash

set -euo pipefail
# Required for signal propagation to work so
# the cleanup trap gets executed when the script
# receives a SIGINT
set -o monitor

cd $(go env GOPATH)/src/github.com/kubermatic/kubermatic

source ./api/hack/lib.sh

export BUILD_ID=${BUILD_ID:-BUILD_ID_UNDEF}
echodate "Build ID is $BUILD_ID"
export VERSIONS=${VERSIONS_TO_TEST:-"v1.12.4"}
export NAMESPACE="prow-kubermatic-${BUILD_ID}"
echodate "Testing versions: ${VERSIONS}"
export GIT_HEAD_HASH="$(git rev-parse HEAD|tr -d '\n')"
export EXCLUDE_DISTRIBUTIONS=${EXCLUDE_DISTRIBUTIONS:-ubuntu,centos}
export DEFAULT_TIMEOUT_MINUTES=${DEFAULT_TIMEOUT_MINUTES:-10}

# if no provider argument has been specified, default to aws
provider=${PROVIDER:-"aws"}

if [[ -n ${OPENSHIFT:-} ]]; then
  OPENSHIFT_ARG="-openshift=true"
  OPENSHIFT_HELM_ARGS="--set-string=kubermatic.controller.featureGates=OpenIDAuthPlugin=true
 --set-string=kubermatic.auth.caBundle=$(cat /etc/oidc-data/oidc-ca-file|base64 -w0)
 --set-string=kubermatic.auth.tokenIssuer=$OIDC_ISSUER_URL
 --set-string=kubermatic.auth.issuerClientID=$OIDC_ISSUER_CLIENT_ID
 --set-string=kubermatic.auth.issuerClientSecret=$OIDC_ISSUER_CLIENT_SECRET"
fi

function cleanup {
  testRC=$?

  echodate "Starting cleanup"
  set +e

  # Try being a little helpful
  if [[ ${testRC} -ne 0 ]]; then
    echodate "tests failed, describing cluster"

    # Describe cluster
    if [[ $provider == "aws" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Secret Access Key|Access Key Id'
    elif [[ $provider == "packet" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'APIKey|ProjectID'
    elif [[ $provider == "gcp" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Service Account'
    elif [[ $provider == "azure" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'ClientID|ClientSecret|SubscriptionID|TenantID'
    elif [[ $provider == "digitalocean" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Token'
    elif [[ $provider == "hetzner" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Token'
    elif [[ $provider == "openstack" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Domain|Tenant|Username|Password'
    elif [[ $provider == "vsphere" ]]; then
      kubectl describe cluster -l worker-name=$BUILD_ID|egrep -vi 'Username|Password'
    else
      echo "Provider $provider is not yet supported."
      exit 1
    fi

    # Control plane logs
    echodate "Dumping all conntrol plane logs"
    local GOTEMPLATE='{{ range $pod := .items }}{{ range $container := .spec.containers }}{{ printf "%s,%s\n" $pod.metadata.name $container.name }}{{end}}{{end}}'
    for i in $(kubectl get pods -n $NAMESPACE -o go-template="$GOTEMPLATE"); do
      local POD="${i%,*}"
      local CONTAINER="${i#*,}"

      echo " [*] Pod $POD, container $CONTAINER:"
      kubectl logs -n $NAMESPACE "$POD" "$CONTAINER"
    done

    # Display machine events, we don't have to worry about secrets here as they are stored in the machine-controllers env
    # Except for vSphere
    TMP_KUBECONFIG=$(mktemp);
    USERCLUSTER_NS=$(kubectl get cluster -o name -l worker-name=${BUILD_ID} |sed 's#.kubermatic.k8s.io/#-#g')
    kubectl get secret -n ${USERCLUSTER_NS} admin-kubeconfig -o go-template='{{ index .data "kubeconfig" }}' | base64 -d > $TMP_KUBECONFIG
    kubectl --kubeconfig=${TMP_KUBECONFIG} describe machine -n kube-system|egrep -vi 'password|user'
  fi

  # Delete addons from all clusters that have our worker-name label
  kubectl get cluster -l worker-name=$BUILD_ID \
     -o go-template='{{range .items}}{{.metadata.name}}{{end}}' \
     |xargs -n 1 -I ^ kubectl label addon -n cluster-^ --all worker-name-

  # Delete all clusters that have our worker-name label
  kubectl delete cluster -l worker-name=$BUILD_ID --wait=false

  # Remove the worker-name label from all clusters that have our worker-name
  # label so the main cluster-controller will clean them up
  kubectl get cluster -l worker-name=$BUILD_ID \
    -o go-template='{{range .items}}{{.metadata.name}}{{end}}' \
      |xargs -I ^ kubectl label cluster ^ worker-name-

  # Delete the Helm Deployment of Kubermatic
  helm delete --purge kubermatic-$BUILD_ID  \
    --tiller-namespace=$NAMESPACE

  # Delete the Helm installation
  kubectl delete clusterrolebinding -l prowjob=$BUILD_ID
  kubectl delete namespace $NAMESPACE --wait=false

  # Upload the JUNIT files
  mv /reports/* ${ARTIFACTS}/
  echodate "Finished cleanup"
}
trap cleanup EXIT

echodate "Getting secrets from Vault"
export VAULT_ADDR=https://vault.loodse.com/
export VAULT_TOKEN=$(vault write \
  --format=json auth/approle/login \
  role_id=$VAULT_ROLE_ID secret_id=$VAULT_SECRET_ID \
  | jq .auth.client_token -r)
export KUBECONFIG=/tmp/kubeconfig
export VALUES_FILE=/tmp/values.yaml
export DATACENTERS_FILE=/tmp/datacenters.yaml
cat <<EOF > $DATACENTERS_FILE
datacenters:
#==================================
#===============Seed===============
#==================================
  'prow-build-cluster': #master
    location: Helsinki
    country: FL
    is_seed: true
    spec:
      bringyourown: {}
#==================================
#===========BringYourOwn===========
#==================================
  'byo-prow-build-cluster':
    location: Helsinki
    seed: 'prow-build-cluster'
    country: FL
    spec:
      bringyourown: {}
#==================================
#===========Digitalocean===========
#==================================
  do-ams3:
    location: Amsterdam
    seed: 'prow-build-cluster'
    country: NL
    spec:
      digitalocean:
        region: ams3
  do-nyc1:
    location: New York
    seed: 'prow-build-cluster'
    country: US
    spec:
      digitalocean:
        region: nyc1
  do-sfo2:
    location: San Francisco
    seed: 'prow-build-cluster'
    country: US
    spec:
      digitalocean:
        region: sfo2
  do-sgp1:
    location: Singapore
    seed: 'prow-build-cluster'
    country: SG
    spec:
      digitalocean:
        region: sgp1
  do-lon1:
    location: London
    seed: 'prow-build-cluster'
    country: GB
    spec:
      digitalocean:
        region: lon1
  do-fra1:
    location: Frankfurt
    seed: 'prow-build-cluster'
    country: DE
    spec:
      digitalocean:
        region: fra1
  do-tor1:
    location: Toronto
    seed: 'prow-build-cluster'
    country: CA
    spec:
      digitalocean:
        region: tor1
  do-blr1:
    location: Bangalore
    seed: 'prow-build-cluster'
    country: IN
    spec:
      digitalocean:
        region: blr1
#==================================
#===============AWS================
#==================================
  aws-us-east-1a:
    location: US East (N. Virginia)
    seed: 'prow-build-cluster'
    country: US
    spec:
      aws:
        region: us-east-1
        zone_character: a
  aws-us-east-2a:
    location: US East (Ohio)
    seed: 'prow-build-cluster'
    country: US
    spec:
      aws:
        region: us-east-2
        zone_character: a
  aws-us-west-1b:
    location: US West (N. California)
    seed: 'prow-build-cluster'
    country: US
    spec:
      aws:
        region: us-west-1
        zone_character: b
  aws-us-west-2a:
    location: US West (Oregon)
    seed: 'prow-build-cluster'
    country: US
    spec:
      aws:
        region: us-west-2
        zone_character: a
  aws-ca-central-1a:
    location: Canada (Central)
    seed: 'prow-build-cluster'
    country: CA
    spec:
      aws:
        region: ca-central-1
        zone_character: a
  aws-eu-west-1a:
    location: EU (Ireland)
    seed: 'prow-build-cluster'
    country: IE
    spec:
      aws:
        region: eu-west-1
        zone_character: a
  aws-eu-central-1a:
    location: EU (Frankfurt)
    seed: 'prow-build-cluster'
    country: DE
    spec:
      aws:
        region: eu-central-1
        zone_character: a
  aws-eu-west-2a:
    location: EU (London)
    seed: 'prow-build-cluster'
    country: GB
    spec:
      aws:
        region: eu-west-2
        zone_character: a
  aws-ap-northeast-1a:
    location: Asia Pacific (Tokyo)
    seed: 'prow-build-cluster'
    country: JP
    spec:
      aws:
        region: ap-northeast-1
        zone_character: a
  aws-ap-northeast-2a:
    location: Asia Pacific (Seoul)
    seed: 'prow-build-cluster'
    country: KR
    spec:
      aws:
        region: ap-northeast-2
        zone_character: a
  aws-ap-southeast-1a:
    location: Asia Pacific (Singapore)
    seed: 'prow-build-cluster'
    country: SG
    spec:
      aws:
        region: ap-southeast-1
        zone_character: a
  aws-ap-southeast-2a:
    location: Asia Pacific (Sydney)
    seed: 'prow-build-cluster'
    country: AU
    spec:
      aws:
        region: ap-southeast-2
        zone_character: a
  aws-ap-south-1a:
    location: Asia Pacific (Mumbai)
    seed: 'prow-build-cluster'
    country: IN
    spec:
      aws:
        region: ap-south-1
        zone_character: a
  aws-sa-east-1a:
    location: South America (São Paulo)
    seed: 'prow-build-cluster'
    country: BR
    spec:
      aws:
        region: sa-east-1
        zone_character: a
#==================================
#=============Hetzner==============
#==================================
  hetzner-fsn1:
    location: Falkenstein 1 DC 8
    seed: 'prow-build-cluster'
    country: DE
    spec:
      hetzner:
        datacenter: fsn1-dc8
  hetzner-nbg1:
    location: Nuremberg 1 DC 3
    seed: 'prow-build-cluster'
    country: DE
    spec:
      hetzner:
        datacenter: nbg1-dc3
#==================================
#=============vSphere==============
#==================================
  vsphere-ger:
    location: Hetzner
    seed: 'prow-build-cluster'
    country: DE
    spec:
      vsphere:
        endpoint: "https://vcenter.loodse.com"
        datacenter: "dc-1"
        datastore: "exsi-nas"
        cluster: "cl-1"
        root_path: "/dc-1/vm/e2e-tests"
        templates:
          ubuntu: "machine-controller-e2e-ubuntu"
          centos: "machine-controller-e2e-centos"
          coreos: "machine-controller-e2e-coreos"
#==================================
#============= Azure ==============
#==================================
  azure-westeurope:
    location: "Azure West europe"
    seed: 'prow-build-cluster'
    country: NL
    spec:
      azure:
        location: "westeurope"
  azure-eastus:
    location: "Azure East US"
    seed: 'prow-build-cluster'
    country: US
    spec:
      azure:
        location: "eastus"
  azure-southeastasia:
    location: "Azure South-East Asia"
    seed: 'prow-build-cluster'
    country: HK
    spec:
      azure:
        location: "southeastasia"
#==================================
#============= GCP ================
#==================================
  gcp-westeurope:
    location: "Europe West (Germany)"
    seed: 'prow-build-cluster'
    country: DE
    spec:
      gcp:
        region: europe-west3
        zone_suffixes:
        - c
#==================================
#============= Packet ================
#==================================
  packet-ams1:
    location: "Packet AMS1 (Amsterdam)"
    seed: 'prow-build-cluster'
    country: NL
    spec:
      packet:
        facilities:
        - ams1
#==================================
#============OpenStack=============
#==================================
  syseleven-dbl1:
    location: Syseleven - dbl1
    seed: europe-west3-c
    country: DE
    spec:
      openstack:
        auth_url: https://keystone.cloud.syseleven.net:5000/v3
        availability_zone: dbl1
        region: dbl
        dns_servers:
        - 37.123.105.116
        - 37.123.105.117
        images:
          ubuntu: "kubermatic-e2e-ubuntu"
          centos: "kubermatic-e2e-centos"
          coreos: "kubermatic-e2e-coreos"
        enforce_floating_ip: true
EOF
retry 5 vault kv get -field=kubeconfig \
  dev/seed-clusters/ci.kubermatic.io > $KUBECONFIG
retry 5 vault kv get -field=values.yaml \
  dev/seed-clusters/ci.kubermatic.io > $VALUES_FILE
sed -E "s/(datacenters: ).*/\1$(base64 -w0 $DATACENTERS_FILE)/" -i $VALUES_FILE
retry 5 vault kv get -field=project_id \
	dev/seed-clusters/ci.kubermatic.io > /tmp/kubermatic_project_id
export KUBERMATIC_PROJECT_ID="$(cat /tmp/kubermatic_project_id)"
retry 5 vault kv get -field=serviceaccount_token \
	dev/seed-clusters/ci.kubermatic.io > /tmp/kubermatic_serviceaccount_token
export KUBERMATIC_SERVICEACCOUNT_TOKEN="$(cat /tmp/kubermatic_serviceaccount_token)"
echodate "Successfully got secrets from Vault"


build_tag_if_not_exists() {
  # Build kubermatic binaries and push the image
  if ! curl -Ss --fail "http://registry.registry.svc.cluster.local.:5000/v2/kubermatic/api/tags/list"|grep -q "$1"; then
    mkdir -p /etc/containers
		cat <<EOF > /etc/containers/registries.conf
[registries.search]
registries = ['docker.io']
[registries.insecure]
registries = ["registry.registry.svc.cluster.local:5000"]
EOF
    echodate "Building binaries"
    time make -C api build
    (
      echodate "Building docker image"
      cd api
      time retry 5 buildah build-using-dockerfile --squash -t "registry.registry.svc.cluster.local:5000/kubermatic/api:$1" .
    )
    (
      echodate "Building addons image"
      cd addons
      time retry 5 buildah build-using-dockerfile --squash -t "registry.registry.svc.cluster.local:5000/kubermatic/addons:$1" .
    )
    echodate "Pushing docker image"
    time retry 5 buildah push "registry.registry.svc.cluster.local:5000/kubermatic/api:$1"
    echodate "Pushing addons image"
    time retry 5 buildah push "registry.registry.svc.cluster.local:5000/kubermatic/addons:$1"
    echodate "Finished building and pushing docker image"
  else
    echodate "Omitting building of binaries and docker image, as tag $1 already exists in local registry"
  fi
}

build_tag_if_not_exists "$GIT_HEAD_HASH"

INITIAL_MANIFESTS=$(cat <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: $NAMESPACE
spec: {}
status: {}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tiller
  namespace: $NAMESPACE
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prow-${BUILD_ID}-tiller
  labels:
    prowjob: "${BUILD_ID}"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: tiller
    namespace: $NAMESPACE
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: prow-${BUILD_ID}-kubermatic
  labels:
    prowjob: "${BUILD_ID}"
subjects:
- kind: ServiceAccount
  name: kubermatic
  namespace: $NAMESPACE
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
EOF
)
echodate "Creating namespace $NAMESPACE to deploy kubermatic in"
echo "$INITIAL_MANIFESTS"|kubectl apply -f -

echodate "Deploying tiller"
helm init --wait --service-account=tiller --tiller-namespace=$NAMESPACE

echodate "Installing Kubermatic via Helm"

if [[ -n ${UPGRADE_TEST_BASE_HASH:-} ]]; then
  echodate "Upgradetest, checking out revision ${UPGRADE_TEST_BASE_HASH}"
  git checkout $UPGRADE_TEST_BASE_HASH
  build_tag_if_not_exists "$UPGRADE_TEST_BASE_HASH"
fi
# We must delete all templates for cluster-scoped resources
# because those already exist because of the main Kubermatic installation
# otherwise the helm upgrade --install fails
rm -f config/kubermatic/templates/cluster-role-binding.yaml
rm -f config/kubermatic/templates/vpa-*
# --force is needed in case the first attempt at installing didn't succeed
# see https://github.com/helm/helm/pull/3597
retry 3 helm upgrade --install --force --wait --timeout 300 \
  --tiller-namespace=$NAMESPACE \
  --set=kubermatic.isMaster=true \
  --set-string=kubermatic.controller.addons.kubernetes.image.tag=${UPGRADE_TEST_BASE_HASH:-$GIT_HEAD_HASH} \
  --set-string=kubermatic.controller.addons.kubernetes.image.repository=127.0.0.1:5000/kubermatic/addons \
  --set-string=kubermatic.controller.image.tag=${UPGRADE_TEST_BASE_HASH:-$GIT_HEAD_HASH} \
  --set-string=kubermatic.controller.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.api.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.api.image.tag=${UPGRADE_TEST_BASE_HASH:-$GIT_HEAD_HASH} \
  --set-string=kubermatic.masterController.image.tag=${UPGRADE_TEST_BASE_HASH:-$GIT_HEAD_HASH} \
  --set-string=kubermatic.masterController.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.kubermaticImage=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.worker_name=$BUILD_ID \
  --set=kubermatic.ingressClass=non-existent \
  --set=kubermatic.checks.crd.disable=true \
  ${OPENSHIFT_HELM_ARGS:-} \
  --values ${VALUES_FILE} \
  --namespace $NAMESPACE \
  kubermatic-$BUILD_ID ./config/kubermatic/


echodate "Finished installing Kubermatic"

# We build the CLI after deploying to make sure we fail fast if the helm deployment fails
echodate "Building conformance-tests cli"
time go build -v github.com/kubermatic/kubermatic/api/cmd/conformance-tests
echodate "Finished building conformance-tests cli"

if [[ -n ${UPGRADE_TEST_BASE_HASH:-} ]]; then
  echodate "Upgradetest, going back to old revision"
  git checkout -
fi

echodate "Starting conformance tests"
export KUBERMATIC_APISERVER_ADDRESS="kubermatic-api.prow-kubermatic-${BUILD_ID}.svc.cluster.local.:80"
if [[ $provider == "aws" ]]; then
  EXTRA_ARGS="-aws-access-key-id=${AWS_E2E_TESTS_KEY_ID}
     -aws-secret-access-key=${AWS_E2E_TESTS_SECRET}"
elif [[ $provider == "packet" ]]; then
  EXTRA_ARGS="-packet-api-key=${PACKET_API_KEY}
     -packet-project-id=${PACKET_PROJECT_ID}"
elif [[ $provider == "gcp" ]]; then
  EXTRA_ARGS="-gcp-service-account=${GOOGLE_SERVICE_ACCOUNT}"
elif [[ $provider == "azure" ]]; then
  EXTRA_ARGS="-azure-client-id=${AZURE_E2E_TESTS_CLIENT_ID}
    -azure-client-secret=${AZURE_E2E_TESTS_CLIENT_SECRET}
    -azure-tenant-id=${AZURE_E2E_TESTS_TENANT_ID}
    -azure-subscription-id=${AZURE_E2E_TESTS_SUBSCRIPTION_ID}"
elif [[ $provider == "digitalocean" ]]; then
  EXTRA_ARGS="-digitalocean-token=${DO_E2E_TESTS_TOKEN}"
elif [[ $provider == "hetzner" ]]; then
  EXTRA_ARGS="-hetzner-token=${HZ_E2E_TOKEN}"
elif [[ $provider == "openstack" ]]; then
  EXTRA_ARGS="-openstack-domain=${OS_DOMAIN}
    -openstack-tenant=${OS_TENANT_NAME}
    -openstack-username=${OS_USERNAME}
    -openstack-password=${OS_PASSWORD}"
elif [[ $provider == "vsphere" ]]; then
  EXTRA_ARGS="-vsphere-username=${VSPHERE_E2E_USERNAME}
    -vsphere-password=${VSPHERE_E2E_PASSWORD}"
fi

timeout -s 9 90m ./conformance-tests $EXTRA_ARGS \
  -debug \
  -worker-name=$BUILD_ID \
  -kubeconfig=$KUBECONFIG \
  -datacenters=$DATACENTERS_FILE \
  -kubermatic-nodes=3 \
  -kubermatic-parallel-clusters=1 \
  -name-prefix=prow-e2e \
  -reports-root=/reports \
  -cleanup-on-start=false \
  -run-kubermatic-controller-manager=false \
  -versions="$VERSIONS" \
  -providers=$provider \
  -exclude-distributions="${EXCLUDE_DISTRIBUTIONS}" \
  ${OPENSHIFT_ARG:-} \
  -kubermatic-delete-cluster=false \
  -print-ginkgo-logs=true \
  -default-timeout-minutes=${DEFAULT_TIMEOUT_MINUTES}

# No upgradetest, just exit
if [[ -z ${UPGRADE_TEST_BASE_HASH:-} ]]; then
  echodate "Success!"
  exit 0
fi

echodate "Installing current version of Kubermatic"
retry 3 helm upgrade --install --force --wait --timeout 300 \
  --tiller-namespace=$NAMESPACE \
  --set=kubermatic.isMaster=true \
  --set-string=kubermatic.controller.addons.kubernetes.image.tag=${GIT_HEAD_HASH} \
  --set-string=kubermatic.controller.addons.kubernetes.image.repository=127.0.0.1:5000/kubermatic/addons \
  --set-string=kubermatic.controller.image.tag=${GIT_HEAD_HASH} \
  --set-string=kubermatic.controller.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.api.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.api.image.tag=${GIT_HEAD_HASH} \
  --set-string=kubermatic.masterController.image.tag=${GIT_HEAD_HASH} \
  --set-string=kubermatic.masterController.image.repository=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.kubermaticImage=127.0.0.1:5000/kubermatic/api \
  --set-string=kubermatic.worker_name=$BUILD_ID \
  --set=kubermatic.ingressClass=non-existent \
  --set=kubermatic.checks.crd.disable=true \
  ${OPENSHIFT_HELM_ARGS:-} \
  --values ${VALUES_FILE} \
  --namespace $NAMESPACE \
  kubermatic-$BUILD_ID ./config/kubermatic/
echodate "Successfully installed current version of Kubermatic"

# We have to rebuild it so it is based on the newer Kubermatic
echodate "Building conformance-tests cli"
time go build -v github.com/kubermatic/kubermatic/api/cmd/conformance-tests

echodate "Running conformance tester with existing cluster"

# We increase the number of nodes to make sure creation
# of nodes still work
timeout -s 9 60m ./conformance-tests $EXTRA_ARGS \
  -debug \
  -existing-cluster-label=worker-name=$BUILD_ID \
  -worker-name=$BUILD_ID \
  -kubeconfig=$KUBECONFIG \
  -datacenters=$DATACENTERS_FILE \
  -kubermatic-nodes=5 \
  -kubermatic-parallel-clusters=1 \
  -kubermatic-delete-cluster=true \
  -name-prefix=prow-e2e \
  -reports-root=/reports \
  -cleanup-on-start=false \
  -versions="$VERSIONS" \
  -providers=$provider \
  -exclude-distributions="${EXCLUDE_DISTRIBUTIONS}" \
  ${OPENSHIFT_ARG:-} \
  -kubermatic-delete-cluster=false \
  -print-ginkgo-logs=true \
  -default-timeout-minutes=${DEFAULT_TIMEOUT_MINUTES}
