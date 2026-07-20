#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="aegis-pi-worker:latest"
KALI_BASE_IMAGE="${AEGIS_KALI_BASE_IMAGE:-docker.io/kalilinux/kali-rolling:latest}"

if ! command -v docker >/dev/null 2>&1; then
  echo "错误：未找到 Docker CLI。" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "错误：Docker daemon 不可用，请先启动 Docker。" >&2
  exit 1
fi

if [[ ! -f "${SCRIPT_DIR}/Dockerfile" ]]; then
  echo "错误：项目根目录中没有 Dockerfile。" >&2
  exit 1
fi

echo "正在构建 ${IMAGE_NAME} ..."
echo "Kali 基础镜像：${KALI_BASE_IMAGE}"
docker build \
  --tag "${IMAGE_NAME}" \
  --build-arg "KALI_BASE_IMAGE=${KALI_BASE_IMAGE}" \
  "$@" \
  "${SCRIPT_DIR}"

echo "正在验证镜像 ..."
docker image inspect "${IMAGE_NAME}" >/dev/null
docker run --rm "${IMAGE_NAME}" sh -ec '
  echo "System:  $(grep PRETTY_NAME /etc/os-release | cut -d= -f2- | tr -d \"\\\"\")"
  echo "Node.js: $(node --version)"
  echo "Python:  $(python --version 2>&1)"
  echo "Go:      $(go version)"
  echo "Java:    $(java -version 2>&1 | head -n 1)"
  echo "Pi:      $(pi --version)"
  echo "Browser: $(agent-browser --version)"
'

echo "构建完成：${IMAGE_NAME}"
