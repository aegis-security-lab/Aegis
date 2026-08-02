#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="${AEGIS_WORKER_IMAGE:-aegis-worker:latest}"
KALI_BASE_IMAGE="${AEGIS_KALI_BASE_IMAGE:-docker.io/kalilinux/kali-rolling:latest}"
AGENT_BROWSER_VERSION="${AEGIS_AGENT_BROWSER_VERSION:-latest}"
PLATFORM="${AEGIS_DOCKER_PLATFORM:-}"
VERIFY_IMAGE="true"
DOCKER_BUILD_ARGS=()
DOCKER_BUILD_ARG_COUNT=0

usage() {
  cat <<'EOF'
构建 Aegis AgentCore Worker 沙箱镜像。

用法：
  ./build-worker-image.sh [脚本选项] [Docker build 参数]

脚本选项：
  --image NAME                  镜像名，默认 aegis-worker:latest
  --base-image NAME             Kali 基础镜像
  --agent-browser-version VER   agent-browser 版本，默认 latest
  --platform PLATFORM           构建平台，例如 linux/amd64 或 linux/arm64
  --no-verify                   构建后不启动临时容器验证工具链
  -h, --help                    显示帮助

其他参数会原样传递给 docker build，例如 --no-cache、--pull 或 --progress=plain。

对应环境变量：
  AEGIS_WORKER_IMAGE
  AEGIS_KALI_BASE_IMAGE
  AEGIS_AGENT_BROWSER_VERSION
  AEGIS_DOCKER_PLATFORM
  AEGIS_SKIP_IMAGE_VERIFY=true
EOF
}

require_value() {
  if [[ $# -lt 2 || -z "$2" ]]; then
    echo "错误：$1 需要一个值。" >&2
    usage >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      require_value "$@"
      IMAGE_NAME="$2"
      shift 2
      ;;
    --base-image)
      require_value "$@"
      KALI_BASE_IMAGE="$2"
      shift 2
      ;;
    --agent-browser-version)
      require_value "$@"
      AGENT_BROWSER_VERSION="$2"
      shift 2
      ;;
    --platform)
      require_value "$@"
      PLATFORM="$2"
      shift 2
      ;;
    --no-verify)
      VERIFY_IMAGE="false"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      DOCKER_BUILD_ARGS+=("$@")
      DOCKER_BUILD_ARG_COUNT=$((DOCKER_BUILD_ARG_COUNT + $#))
      break
      ;;
    *)
      DOCKER_BUILD_ARGS+=("$1")
      DOCKER_BUILD_ARG_COUNT=$((DOCKER_BUILD_ARG_COUNT + 1))
      shift
      ;;
  esac
done

case "${AEGIS_SKIP_IMAGE_VERIFY:-false}" in
  1|true|TRUE|yes|YES) VERIFY_IMAGE="false" ;;
esac

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

BUILD_COMMAND=(
  docker build
  --tag "${IMAGE_NAME}"
  --build-arg "KALI_BASE_IMAGE=${KALI_BASE_IMAGE}"
  --build-arg "AGENT_BROWSER_VERSION=${AGENT_BROWSER_VERSION}"
)
if [[ -n "${PLATFORM}" ]]; then
  BUILD_COMMAND+=(--platform "${PLATFORM}")
fi
if (( DOCKER_BUILD_ARG_COUNT > 0 )); then
  BUILD_COMMAND+=("${DOCKER_BUILD_ARGS[@]}")
fi
BUILD_COMMAND+=("${SCRIPT_DIR}")

echo "正在构建 Worker 镜像"
echo "  镜像：${IMAGE_NAME}"
echo "  基础镜像：${KALI_BASE_IMAGE}"
echo "  agent-browser：${AGENT_BROWSER_VERSION}"
if [[ -n "${PLATFORM}" ]]; then
  echo "  平台：${PLATFORM}"
fi
"${BUILD_COMMAND[@]}"

docker image inspect "${IMAGE_NAME}" >/dev/null
if [[ "${VERIFY_IMAGE}" == "true" ]]; then
  echo "正在验证镜像中的运行环境 ..."
  docker run --rm "${IMAGE_NAME}" sh -ec '
    echo "System:  $(grep PRETTY_NAME /etc/os-release | cut -d= -f2- | tr -d "\"")"
    echo "Node.js: $(node --version)"
    echo "Python:  $(python --version 2>&1)"
    echo "Go:      $(go version)"
    echo "SQLite:  $(sqlite3 --version | cut -d" " -f1)"
    echo "pnpm:    $(pnpm --version)"
    echo "Yarn:    $(yarn --version)"
    echo "Git:     $(git --version)"
    echo "rg:      $(rg --version | head -n 1)"
    echo "Java:    $(java -version 2>&1 | head -n 1)"
    echo "Browser: $(agent-browser --version)"
  '
else
  echo "已跳过运行环境验证。"
fi

echo "构建完成：${IMAGE_NAME}"
