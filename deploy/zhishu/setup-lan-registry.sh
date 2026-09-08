#!/usr/bin/env bash
# ============================================================================
# 知枢(zhishu) 局域网私有镜像仓库初始化 —— 在服务器(10.0.3.109)执行一次
#
# 作用：跑一个 registry:2 容器 zhishu-registry，供开发机 push / 服务器 pull
#       app、ui 两个自建镜像（全走内网，不依赖公网云仓库）。
#       数据落在 /home/zhishu/data/registry（大分区）。
#
# 前置：服务器能访问 Docker Hub 拉 registry:2（国内拉不动就先配 daemon 加速）。
# ============================================================================
set -euo pipefail

BIND_HOST="${ZHISHU_REGISTRY_BIND:-10.0.3.109}"
BIND_PORT="${ZHISHU_REGISTRY_PORT:-5000}"
DATA_DIR="${ZHISHU_REGISTRY_DATA:-/home/zhishu/data/registry}"

if docker ps -a --format '{{.Names}}' | grep -qx zhishu-registry; then
  echo "zhishu-registry 已存在："
  docker ps --filter name=zhishu-registry --format '{{.Names}}  {{.Status}}  {{.Ports}}'
  exit 0
fi

mkdir -p "$DATA_DIR"

echo "==> 启动 zhishu-registry : ${BIND_HOST}:${BIND_PORT} -> /var/lib/registry (数据在 $DATA_DIR)"
docker run -d --name zhishu-registry --restart unless-stopped \
  -p "${BIND_HOST}:${BIND_PORT}:5000" \
  -v "${DATA_DIR}:/var/lib/registry" \
  registry:2

echo "完成。接下来必须让两端的 docker daemon 信任这个 HTTP 仓库："
echo "  1) 服务器 /etc/docker/daemon.json 增加  \"insecure-registries\": [\"${BIND_HOST}:${BIND_PORT}\"]  后 systemctl restart docker"
echo "  2) 开发机 Docker Desktop → Settings → Docker Engine → 加同一项 → Apply & Restart"
echo "  3) 放行端口（仅内网）："
echo "     firewall-cmd --permanent --add-rich-rule='rule family=ipv4 source address=10.0.3.0/24 port port=${BIND_PORT} protocol=tcp accept' && firewall-cmd --reload"
echo "自检（开发机，busybox 需能拉 Docker Hub 或已本地有）："
echo "  docker pull busybox && docker tag busybox ${BIND_HOST}:${BIND_PORT}/zhishu/smoke:1 \\"
echo "    && docker push ${BIND_HOST}:${BIND_PORT}/zhishu/smoke:1"
