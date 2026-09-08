#!/usr/bin/env bash
# ============================================================================
# 知枢(zhishu) 生产 .env 生成器 —— 在服务器部署目录运行
#
# 前置：同目录需有仓库根文件：docker-compose.yml、config/config.yaml、.env.example
#       以及本目录的 docker-compose.override.yml（均已复制到服务器部署目录）
#
# 用法：
#   ./gen-env.sh            # 首次生成 .env（全随机密钥）
#   ./gen-env.sh --force    # 重新生成（会重置全部密钥，慎重）
#
# 说明：
#   - .env 以仓库 .env.example 为全量底稿，再套用下方“生产覆盖”；
#   - 覆盖项保证单键唯一（同名键取最后一次赋值）；
#   - 生成结束会打印 bootstrap 管理员密码，请立即抄入密码管理器；
#   - SYSTEM_AES_KEY 丢失 = 库内已加密字段永久不可解。
# ============================================================================
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

BASE=.env.example
OUT=.env

[ -f "$BASE" ] || { echo "缺少 $BASE —— 先从仓库根复制 .env.example 到本目录"; exit 1; }
if [ -f "$OUT" ] && [ "${1:-}" != "--force" ]; then
  echo "$OUT 已存在。若要按 .env.example 重置并重新随机全部密钥，请加 --force（会覆盖现有配置）。"
  exit 1
fi

# ============================ 可配置（首次修改这里） ============================
# 镜像仓库 —— 与 ./build-and-push.sh 保持一致
#   阿里云 ACR 个人版：  registry.cn-hangzhou.aliyuncs.com
#   腾讯云 TCR 个人版：  ccr.ccs.tencentcloud.com
REG_ENDPOINT="${ZHISHU_REG_ENDPOINT:-registry.cn-hangzhou.aliyuncs.com}"
REG_NAMESPACE="${ZHISHU_REG_NAMESPACE:-zhishu}"     # 控制台里建的命名空间
TAG="${ZHISHU_TAG:-$(date +%Y%m%d)}"                # 与本机 build-and-push.sh 的 TAG 一致

# 业务库 / MinIO 桶
DB_USER="${ZHISHU_DB_USER:-zhishu}"
DB_NAME="${ZHISHU_DB_NAME:-zhishu}"
MINIO_BUCKET="${ZHISHU_MINIO_BUCKET:-zhishu}"

# 端口（局域网直连入口 / 本机调试口 / minio 仅回环）
FRONTEND_PORT="${ZHISHU_FRONTEND_PORT:-8088}"      # 浏览器 http://<服务器IP>:8088
APP_PORT="${ZHISHU_APP_PORT:-127.0.0.1:18080}"     # 仅供本机调试，不对外
# ===============================================================================

ZHISHU_APP_IMAGE="$REG_ENDPOINT/$REG_NAMESPACE/zhishu-app:$TAG"
ZHISHU_UI_IMAGE="$REG_ENDPOINT/$REG_NAMESPACE/zhishu-ui:$TAG"

# ---------------------------- 随机密钥生成 ----------------------------
RAND_HEX() { openssl rand -hex "$1"; }
DB_PASSWORD="$(RAND_HEX 16)"
REDIS_PASSWORD="$(RAND_HEX 12)"
JWT_SECRET="$(RAND_HEX 32)"
SYSTEM_AES_KEY="$(openssl rand -base64 24 | tr -d '\n')"   # 32 字节
MINIO_ACCESS_KEY_ID="$(RAND_HEX 10)"                       # MinIO 限制 ≤20 字符
MINIO_SECRET_ACCESS_KEY="$(RAND_HEX 16)"
ADMIN_PASSWORD="$(openssl rand -base64 18 | tr -dc 'A-Za-z0-9' | cut -c1-20)"

# ---------------------------- 生成 .env ----------------------------
cp -f "$BASE" "$OUT"

# apply KEY VALUE：把 .env 中该键“激活/覆盖/追加”（保留其余注释与默认）
apply() {
  awk -v k="$1" -v v="$2" '
    BEGIN { done = 0 }
    { if (!done && $0 ~ "^[[:space:]#]*" k "=") { print k "=" v; done = 1; next } print }
    END { if (!done) print k "=" v }
  ' "$OUT" > "$OUT.tmp" && mv "$OUT.tmp" "$OUT"
}

# ---- 端口 / 运行时 ----
apply FRONTEND_PORT "$FRONTEND_PORT"
apply APP_PORT "$APP_PORT"
apply GIN_MODE release
apply LOG_LEVEL info

# ---- 数据库 / Redis / 检索 ----
apply DB_USER "$DB_USER"
apply DB_PASSWORD "$DB_PASSWORD"
apply DB_NAME "$DB_NAME"
apply REDIS_PASSWORD "$REDIS_PASSWORD"
apply RETRIEVE_DRIVER postgres

# ---- 密钥 ----
apply JWT_SECRET "$JWT_SECRET"
apply SYSTEM_AES_KEY "$SYSTEM_AES_KEY"

# ---- 对象存储 → MinIO（app 走内网 minio:9000；S3/控制台仅回环不对外） ----
apply STORAGE_TYPE minio
apply MINIO_ENDPOINT minio:9000
apply MINIO_ACCESS_KEY_ID "$MINIO_ACCESS_KEY_ID"
apply MINIO_SECRET_ACCESS_KEY "$MINIO_SECRET_ACCESS_KEY"
apply MINIO_BUCKET_NAME "$MINIO_BUCKET"
apply MINIO_USE_SSL false
apply MINIO_PORT 127.0.0.1:9000
apply MINIO_CONSOLE_PORT 127.0.0.1:9001

# ---- 不使用 LangFuse：清空连接键（.env.example 里是激活的占位值） ----
apply LANGFUSE_PUBLIC_KEY ""
apply LANGFUSE_SECRET_KEY ""
apply LANGFUSE_HOST ""

# ---- bootstrap 管理员（.env.example 中是注释态，需激活） ----
apply WEKNORA_BOOTSTRAP_ADMIN_EMPLOYEE_ID admin
apply WEKNORA_BOOTSTRAP_ADMIN_PASSWORD "$ADMIN_PASSWORD"

# ---- 自建镜像引用（供 docker-compose.override.yml 插值） ----
apply ZHISHU_APP_IMAGE "$ZHISHU_APP_IMAGE"
apply ZHISHU_UI_IMAGE "$ZHISHU_UI_IMAGE"

chmod 600 "$OUT"

echo "已生成 $OUT"
echo "------------------------------------------------------------------------"
echo " 应用镜像 : $ZHISHU_APP_IMAGE"
echo " 前端镜像 : $ZHISHU_UI_IMAGE"
echo " 访问入口 : http://<服务器IP>:$FRONTEND_PORT"
echo " MinIO    : bucket=$MINIO_BUCKET，S3/控制台绑定 127.0.0.1（不对外）"
echo " 管理员账号 : admin"
echo " 管理员密码 : $ADMIN_PASSWORD   <-- 请立即抄入密码管理器"
echo "------------------------------------------------------------------------"
echo "下一步："
echo "  docker login $REG_ENDPOINT"
echo "  docker compose -p zhishu --profile minio pull"
echo "  docker compose -p zhishu --profile minio up -d"
