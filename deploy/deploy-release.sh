#!/usr/bin/env bash
set -Eeuo pipefail

readonly REPOSITORY="karenepitaya/curated-poetry-api"
readonly RELEASE_ROOT="/opt/poetry-api/releases"
readonly CURRENT_LINK="/opt/poetry-api/current"
readonly SERVICE="poetry-api"
readonly LOOPBACK_BASE_URL="http://127.0.0.1:8787"
readonly PUBLIC_BASE_URL="https://poetry-api.karenepitaya.xyz"

if [[ ${EUID} -ne 0 ]]; then
  echo "run this script as root" >&2
  exit 1
fi

if [[ $# -ne 1 || ! $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
fi

readonly TAG="$1"
readonly VERSION_DIR="${RELEASE_ROOT}/${TAG}"
readonly STAGING_DIR="${RELEASE_ROOT}/.${TAG}.staging.$$"
readonly DOWNLOAD_BASE="https://github.com/${REPOSITORY}/releases/download/${TAG}"
readonly TEMP_DIR="$(mktemp -d)"
PREVIOUS_TARGET=""
SWITCHED=0
VERSION_CREATED=0

cleanup() {
  rm -rf -- "${TEMP_DIR}"
  if [[ -d ${STAGING_DIR} ]]; then
    rm -rf -- "${STAGING_DIR}"
  fi
}

switch_current() {
  local target="$1"
  local next_link="/opt/poetry-api/.current.next.$$"
  ln -s "${target}" "${next_link}"
  mv -Tf "${next_link}" "${CURRENT_LINK}"
}

rollback() {
  local status=$?
  local restored=0
  trap - ERR
  set +e
  if [[ ${SWITCHED} -eq 1 && -n ${PREVIOUS_TARGET} && -d ${PREVIOUS_TARGET} ]]; then
    echo "deployment failed; rolling back to ${PREVIOUS_TARGET}" >&2
    if switch_current "${PREVIOUS_TARGET}"; then
      restored=1
      systemctl restart "${SERVICE}"
      curl --fail --silent --show-error --connect-timeout 2 --max-time 5 \
        "${LOOPBACK_BASE_URL}/healthz" >/dev/null
    else
      echo "rollback could not restore ${CURRENT_LINK}; keeping failed release directory for recovery" >&2
    fi
  elif [[ ${SWITCHED} -eq 1 ]]; then
    echo "deployment failed with no previous release; stopping ${SERVICE}" >&2
    systemctl stop "${SERVICE}"
    if [[ -L ${CURRENT_LINK} && $(readlink -f "${CURRENT_LINK}") == "${VERSION_DIR}" ]]; then
      if rm -f -- "${CURRENT_LINK}"; then
        restored=1
      fi
    fi
  fi
  if [[ ${VERSION_CREATED} -eq 1 && -d ${VERSION_DIR} && (${SWITCHED} -eq 0 || ${restored} -eq 1) ]]; then
    rm -rf -- "${VERSION_DIR}"
  fi
  exit "${status}"
}

trap cleanup EXIT
trap rollback ERR

if [[ -e ${VERSION_DIR} || -L ${VERSION_DIR} || -e ${STAGING_DIR} ]]; then
  echo "release directory already exists for ${TAG}; refusing to overwrite it" >&2
  exit 3
fi

for asset in curated-poetry-api curated-poetry-api.sha256 LICENSE DATA_LICENSE NOTICE; do
  curl --fail --location --silent --show-error --retry 3 --retry-delay 1 \
    --connect-timeout 5 --max-time 60 \
    --output "${TEMP_DIR}/${asset}" "${DOWNLOAD_BASE}/${asset}"
done

(
  cd "${TEMP_DIR}"
  sha256sum --check --strict curated-poetry-api.sha256
)

install -d -o root -g root -m 0755 "${STAGING_DIR}"
install -o root -g root -m 0755 "${TEMP_DIR}/curated-poetry-api" "${STAGING_DIR}/curated-poetry-api"
install -o root -g root -m 0644 \
  "${TEMP_DIR}/LICENSE" "${TEMP_DIR}/DATA_LICENSE" "${TEMP_DIR}/NOTICE" "${STAGING_DIR}/"
mv -T "${STAGING_DIR}" "${VERSION_DIR}"
VERSION_CREATED=1

if [[ -L ${CURRENT_LINK} ]]; then
  PREVIOUS_TARGET="$(readlink -f "${CURRENT_LINK}")"
fi

switch_current "${VERSION_DIR}"
SWITCHED=1
systemctl restart "${SERVICE}"

loopback_health="$(curl --fail --silent --show-error --connect-timeout 2 --max-time 10 \
  "${LOOPBACK_BASE_URL}/healthz")"
grep -Fq "\"version\":\"${TAG}\"" <<<"${loopback_health}"
public_health="$(curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  "${PUBLIC_BASE_URL}/healthz")"
grep -Fq '"status":"ok"' <<<"${public_health}"
random_response="$(curl --fail --silent --show-error --connect-timeout 5 --max-time 15 \
  "${PUBLIC_BASE_URL}/api/v1/works/random?max_chars=120&script=hans")"
grep -Fq '"data"' <<<"${random_response}"

SWITCHED=0
trap - ERR

mapfile -t releases < <(
  find "${RELEASE_ROOT}" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' \
    | sort -nr \
    | cut -d' ' -f2-
)
kept_previous=0
for candidate in "${releases[@]}"; do
  if [[ ${candidate} == "${VERSION_DIR}" ]]; then
    continue
  fi
  if [[ ${kept_previous} -lt 2 ]]; then
    kept_previous=$((kept_previous + 1))
    continue
  fi
  case "${candidate}" in
    "${RELEASE_ROOT}"/*) rm -rf -- "${candidate}" ;;
    *) echo "refusing to remove unexpected path: ${candidate}" >&2; exit 1 ;;
  esac
done

echo "deployed ${TAG} successfully"
