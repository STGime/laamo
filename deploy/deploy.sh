#!/usr/bin/env bash
# Deploys Laamo to Scaleway Serverless Containers (Paris), next to Eurobase.
#
#   deploy/deploy.sh staging        (or: task deploy:staging)
#
# Needs: scw (logged in with `scw init`), docker with buildx, jq, and
# deploy/<env>.env (from deploy/env.example). Builds and pushes the images
# tagged with the git commit, then creates or updates three containers in
# the namespace laamo-<env>:
#
#   api     the HTTP API             scales 0..3 (staging) / 1..5 (prod)
#   worker  runs, schedules, outbox  always exactly 1
#   web     the app + /api proxy     scales 0..3 / 1..5
#
# SKIP_BUILD=1 uses images already in the registry (TAG=<commit>).
# ALLOW_DIRTY=1 deploys uncommitted changes (the tag gets "-dirty").
set -euo pipefail

ENV_NAME=${1:-}
case "$ENV_NAME" in staging | prod) ;; *)
	echo "usage: $0 staging|prod" >&2
	exit 2
	;;
esac
cd "$(dirname "$0")/.."

for bin in scw docker jq git openssl; do
	command -v "$bin" >/dev/null || {
		echo "needs $bin" >&2
		exit 1
	}
done

CONF=deploy/$ENV_NAME.env
[[ -f $CONF ]] || {
	echo "missing $CONF: copy deploy/env.example and fill it in" >&2
	exit 1
}
set -a
# shellcheck disable=SC1090
. "$CONF"
set +a
for v in SCW_REGISTRY_NAMESPACE EUROBASE_URL EUROBASE_PROJECT_ID EUROBASE_PUBLIC_KEY EUROBASE_PRIVATE_KEY; do
	[[ -n ${!v:-} ]] || {
		echo "$CONF: $v is empty" >&2
		exit 1
	}
done

# The public site (links in emails point here): WEB_DOMAIN, else laamo.app.
SITE_URL="https://${WEB_DOMAIN:-laamo.app}"
[[ $ENV_NAME == prod || -n ${WEB_DOMAIN:-} ]] || echo "warning: no WEB_DOMAIN: $ENV_NAME's email links point to $SITE_URL" >&2

REGION=fr-par
NS_NAME=laamo-$ENV_NAME
REGISTRY=rg.$REGION.scw.cloud/$SCW_REGISTRY_NAMESPACE
TAG=${TAG:-$(git rev-parse --short HEAD)}
# Only a build takes the working tree; SKIP_BUILD reuses pushed images as tagged.
if [[ ${SKIP_BUILD:-} != 1 && -n $(git status --porcelain) ]]; then
	[[ ${ALLOW_DIRTY:-} == 1 ]] || {
		echo "uncommitted changes: commit first (or ALLOW_DIRTY=1)" >&2
		exit 1
	}
	TAG=$TAG-dirty
fi
if [[ $ENV_NAME == prod ]]; then MIN=1 MAX=5; else MIN=0 MAX=3; fi

say() { printf '\n== %s\n' "$*"; }

# --- images -----------------------------------------------------------------
if [[ ${SKIP_BUILD:-} != 1 ]]; then
	say "build and push $TAG"
	scw registry namespace list name="$SCW_REGISTRY_NAMESPACE" region=$REGION -o json | jq -e 'length > 0' >/dev/null ||
		scw registry namespace create name="$SCW_REGISTRY_NAMESPACE" region=$REGION is-public=false >/dev/null
	scw config get secret-key | docker login "rg.$REGION.scw.cloud" -u nologin --password-stdin >/dev/null
	for svc in api worker; do
		docker buildx build --platform linux/amd64 --push -f deploy/docker/go.Dockerfile \
			--build-arg CMD=$svc -t "$REGISTRY/$svc:$TAG" .
	done
	# The web bundle holds this environment's Eurobase URL and public key,
	# and the site's URL; only prod may be indexed by search engines.
	SITE_INDEX=
	[[ $ENV_NAME == prod ]] && SITE_INDEX=1
	docker buildx build --platform linux/amd64 --push -f deploy/docker/web.Dockerfile \
		--build-arg EUROBASE_URL="$EUROBASE_URL" --build-arg EUROBASE_PUBLIC_KEY="$EUROBASE_PUBLIC_KEY" \
		--build-arg SITE_URL="$SITE_URL" --build-arg SITE_INDEX="$SITE_INDEX" \
		-t "$REGISTRY/web:$ENV_NAME-$TAG" .
fi

# --- namespace --------------------------------------------------------------
say "namespace $NS_NAME"
NS_ID=$(scw container namespace list name="$NS_NAME" region=$REGION -o json | jq -r '.[0].id // empty')
if [[ -z $NS_ID ]]; then
	NS_ID=$(scw container namespace create name="$NS_NAME" region=$REGION -w -o json | jq -r .id)
fi

# The app's settings: every line of the env file except SCW_* and WEB_*,
# as secret variables (hidden in the console).
SECRETS=()
i=0
while IFS= read -r line; do
	[[ $line =~ ^([A-Z][A-Z0-9_]*)= ]] || continue
	key=${BASH_REMATCH[1]}
	[[ $key == SCW_* || $key == WEB_* ]] && continue
	val=${!key:-}
	[[ -z $val ]] && continue
	SECRETS+=("secret-environment-variables.$i.key=$key" "secret-environment-variables.$i.value=$val")
	i=$((i + 1))
done <"$CONF"

# upsert <name> <image> <memory MB> <min> <max> <health path> [arg=value ...]:
# creates the container or updates it (which redeploys), waits until it's
# ready and prints its host name.
upsert() {
	local name=$1 image=$2 mem=$3 min=$4 max=$5 health=$6
	shift 6
	# Scaleway pairs CPU with memory: 70 mvCPU per 128 MB.
	local args=(registry-image="$image" memory-limit="$mem" cpu-limit=$((mem * 70 / 128))
		min-scale="$min" max-scale="$max" port=8080 privacy=public http-option=redirected
		health-check.http.path="$health" region=$REGION "$@"
		# Setting only the path makes the API reject the health check; these are
		# Scaleway's defaults (30 failures in a row, checked every 10s).
		health-check.failure-threshold=30 health-check.interval=10s)
	local id out
	id=$(scw container container list namespace-id="$NS_ID" name="$name" region=$REGION -o json | jq -r '.[0].id // empty')
	if [[ -z $id ]]; then
		out=$(scw container container create namespace-id="$NS_ID" name="$name" "${args[@]}" deploy=true -w -o json)
	else
		out=$(scw container container update "$id" "${args[@]}" -w -o json)
	fi
	jq -r '"\(.name): \(.status)"' <<<"$out" >&2
	[[ $(jq -r .status <<<"$out") == ready ]] || {
		echo "$name is not ready: $(jq -r '.error_message // empty' <<<"$out")" >&2
		exit 1
	}
	jq -r .domain_name <<<"$out"
}

say "api"
API_HOST=$(upsert api "$REGISTRY/api:$TAG" 512 "$MIN" "$MAX" /api/healthz timeout=900s \
	environment-variables.LAAMO_ENV="$ENV_NAME" environment-variables.API_ADDR=:8080 \
	environment-variables.SITE_URL="$SITE_URL" "${SECRETS[@]}")

say "worker"
upsert worker "$REGISTRY/worker:$TAG" 1024 1 1 /healthz \
	environment-variables.LAAMO_ENV="$ENV_NAME" environment-variables.HEALTH_ADDR=:8080 \
	environment-variables.SITE_URL="$SITE_URL" "${SECRETS[@]}" >/dev/null

# A login in front of the web container (staging): WEB_LOGIN_USER and
# WEB_LOGIN_PASSWORD in the env file. Only the hash leaves this machine.
WEB_LOGIN=()
if [[ -n ${WEB_LOGIN_USER:-} && -n ${WEB_LOGIN_PASSWORD:-} ]]; then
	hash=$(printf '%s' "$WEB_LOGIN_PASSWORD" | openssl passwd -apr1 -stdin)
	WEB_LOGIN=("secret-environment-variables.0.key=BASIC_AUTH_HTPASSWD"
		"secret-environment-variables.0.value=$WEB_LOGIN_USER:$hash")
elif [[ $ENV_NAME != prod ]]; then
	echo "warning: no WEB_LOGIN_USER/WEB_LOGIN_PASSWORD: $ENV_NAME is open to everyone" >&2
fi

say "web"
WEB_HOST=$(upsert web "$REGISTRY/web:$ENV_NAME-$TAG" 128 "$MIN" "$MAX" /healthz timeout=900s \
	environment-variables.API_UPSTREAM="https://$API_HOST" ${WEB_LOGIN[@]+"${WEB_LOGIN[@]}"})

if [[ -n ${WEB_DOMAIN:-} ]]; then
	WEB_ID=$(scw container container list namespace-id="$NS_ID" name=web region=$REGION -o json | jq -r '.[0].id')
	if ! scw container domain list container-id="$WEB_ID" region=$REGION -o json | jq -e --arg h "$WEB_DOMAIN" 'any(.[]; .hostname == $h)' >/dev/null; then
		say "domain $WEB_DOMAIN (its CNAME must point to $WEB_HOST)"
		scw container domain create container-id="$WEB_ID" hostname="$WEB_DOMAIN" region=$REGION >/dev/null
	fi
fi

say "health"
curl -fsS "https://$WEB_HOST/api/healthz" >/dev/null && echo "api ok through web"
echo
echo "web:  https://${WEB_DOMAIN:-$WEB_HOST}"
echo "api:  https://$API_HOST"
echo "tag:  $TAG"
