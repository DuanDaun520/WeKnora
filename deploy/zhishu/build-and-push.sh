#!/usr/bin/env bash
# ============================================================================
# 知枢(zhishu) 镜像本机构建并推送 —— 在开发机(Mac)仓库根下运行
#
# 产出：linux/amd64 的 zhishu-app / zhishu-ui 两个镜像，推送到国内云镜像仓库，
#       供局域网 CentOS 服务器 pull（x86_64）。
#
# 前置：
#   - Docker + buildx；Docker daemon 已配国内镜像加速（base 镜像直连 Docker Hub 会挂）
#   - 能访问国内云镜像仓库，已 docker login
#   - node/pnpm 环境（构建前端 dist）
#
# 用法：
#   ZHISHU_REG=registry.cn-hangzhou.aliyuncs.com/zhishu ./build-and-push.sh
#   ZHISHU_REG=ccr.ccs.tencentcloud.com/zhishu TAG=20260908 ./build-and-push.sh
#
# TAG 默认当天日期，务必与服务器端 gen-env.sh 的 ZHISHU_TAG 保持一致。
# ============================================================================
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

# ============================ 可配置 ============================
# 例：阿里云 ACR  ->  registry.cn-hangzhou.aliyuncs.com/zhishu
#     腾讯云 TCR  ->  ccr.ccs.tencentcloud.com/zhishu
ZHISHU_REG="${ZHISHU_REG:-}"
TAG="${TAG:-$(date +%Y%m%d)}"
# ================================================================

[ -n "$ZHISHU_REG" ] || { echo "请设置 ZHISHU_REG，例如 ZHISHU_REG=registry.cn-hangzhou.aliyuncs.com/zhishu"; exit 1; }
command -v docker >/dev/null || { echo "缺少 docker"; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker daemon 未运行"; exit 1; }

# buildx 容器驱动 builder（支持跨平台构建与 --push）
if ! docker buildx inspect zhishu-builder >/dev/null 2>&1; then
  echo "==> 创建 buildx builder zhishu-builder ..."
  docker buildx create --name zhishu-builder --driver docker-container --use
fi
docker buildx use zhishu-builder

docker login "$ZHISHU_REG"

# 版本元数据（对齐 scripts/build_images.sh）
V="$(cat VERSION 2>/dev/null | tr -d '\r\n' || echo unknown)"
C="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
T="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
G="$(go version 2>/dev/null || echo unknown)"

echo "==> [1/2] 构建前端静态资源 dist ..."
VITE_FRONTEND_COMMIT="$C" ./scripts/build_frontend_dist.sh

echo "==> [2/2] buildx 构建并推送 linux/amd64 镜像（app / ui）..."
docker buildx build --platform linux/amd64 \
  --build-arg GOPRIVATE_ARG= \
  --build-arg GOPROXY_ARG=https://goproxy.cn,direct \
  --build-arg GOSUMDB_ARG=off \
  --build-arg VERSION_ARG="$V" \
  --build-arg COMMIT_ID_ARG="$C" \
  --build-arg BUILD_TIME_ARG="$T" \
  --build-arg GO_VERSION_ARG="$G" \
  --build-arg WITH_ANYDOC=1 \
  --build-arg APK_MIRROR_ARG= \
  -f docker/Dockerfile.app \
  -t "$ZHISHU_REG/zhishu-app:$TAG" --push .

docker buildx build --platform linux/amd64 \
  -f frontend/Dockerfile \
  -t "$ZHISHU_REG/zhishu-ui:$TAG" --push frontend/

echo "完成。两个镜像已推送："
echo "  $ZHISHU_REG/zhishu-app:$TAG"
echo "  $ZHISHU_REG/zhishu-ui:$TAG"
echo "服务器端（保持 TAG=$TAG）："
echo "  docker login ${ZHISHU_REG%/*}"
echo "  cd /opt/zhishu && docker compose -p zhishu --profile minio pull"
echo "  docker compose -p zhishu --profile minio up -d"
