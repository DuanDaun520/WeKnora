#!/usr/bin/env bash
# ============================================================================
# 知枢(zhishu) 镜像本机构建并推送 —— 在开发机(Mac)仓库根下运行
#
# 产出：linux/amd64 的 zhishu-app / zhishu-ui，推送到局域网私有镜像仓库，
#       供 CentOS Stream 9 服务器（10.0.3.109）pull。
#
# 前置：
#   - Docker + buildx；Docker daemon 已配国内镜像加速（base 镜像直连 Docker Hub 会挂）
#   - 本机 daemon 已加 insecure-registries=["10.0.3.109:5000"]（HTTP 私有仓库必需）并重启
#   - 服务器已起 zhishu-registry（见 setup-lan-registry.sh）
#   - node/pnpm 环境（构建前端 dist）
#
# 用法：
#   ./build-and-push.sh
#   改仓库：ZHISHU_REG=<host:port>/<ns> ./build-and-push.sh
#   需认证仓库（如 ACR/TCR）：ZHISHU_REG_USER=xxx ZHISHU_REG_PASS=xxx ZHISHU_REG=... ./build-and-push.sh
#
# TAG 默认当天日期，务必与服务器端 gen-env.sh 的 ZHISHU_TAG 保持一致。
# ============================================================================
set -euo pipefail
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

# ============================ 可配置 ============================
# 默认：局域网自建私有 registry（HTTP、无需登录）
ZHISHU_REG="${ZHISHU_REG:-10.0.3.109:5000/zhishu}"
# 若改用国内云镜像仓库（如 ACR/TCR），示例：
#   ZHISHU_REG_USER=阿里云账号  ZHISHU_REG_PASS=独立登录密码 \
#   ZHISHU_REG=registry.cn-hangzhou.aliyuncs.com/zhishu ./build-and-push.sh
ZHISHU_REG_USER="${ZHISHU_REG_USER:-}"
ZHISHU_REG_PASS="${ZHISHU_REG_PASS:-}"
TAG="${TAG:-$(date +%Y%m%d)}"
# ================================================================

command -v docker >/dev/null || { echo "缺少 docker"; exit 1; }
docker info >/dev/null 2>&1 || { echo "Docker daemon 未运行"; exit 1; }

# buildx 容器驱动 builder（支持跨平台构建与 --push）
if ! docker buildx inspect zhishu-builder >/dev/null 2>&1; then
  echo "==> 创建 buildx builder zhishu-builder ..."
  docker buildx create --name zhishu-builder --driver docker-container --use
fi
docker buildx use zhishu-builder

if [ -n "$ZHISHU_REG_USER" ] && [ -n "$ZHISHU_REG_PASS" ]; then
  echo "==> docker login ${ZHISHU_REG%%/*}"
  echo "$ZHISHU_REG_PASS" | docker login -u "$ZHISHU_REG_USER" --password-stdin "${ZHISHU_REG%%/*}"
else
  echo "==> 跳过 docker login（局域网 registry 无需认证；用云仓库时请设 ZHISHU_REG_USER/PASS）"
fi

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
echo "  cd /home/zhishu && docker compose -p zhishu --profile minio pull"
echo "  docker compose -p zhishu --profile minio up -d"
