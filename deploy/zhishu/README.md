# 知枢（WeKnora fork）局域网 Docker 部署

面向**局域网 CentOS + 宝塔面板（已装 Docker）**单机部署：浏览器直连 `http://<服务器IP>:8088`，无公网域名/SSL。

- 服务：`postgres(ParadeDB PG17)` + `redis` + `minio` + `app` + `frontend` + `docreader`
- 容器名统一 **`zhishu-*`** 前缀
- 不用 LangFuse、searxng、qdrant/milvus/neo4j/doris、sandbox、mcp 等（保持 profile 不启动即可）
- 模型全部走在线 OpenAI 兼容 API（系统 UI 里配置）

## 一、总体拓扑

```
局域网浏览器
    │  http://<服务器IP>:8088
    ▼
zhishu-frontend  (nginx :80, 容器内)
    │  代理 /api /files /r/
    ▼
zhishu-app       (:8080) ──▶  zhishu-postgres(5432) / zhishu-redis(6379)
    └──────────────▶  zhishu-docreader(50051, gRPC)
    └──────────────▶  zhishu-minio(9000)   # 仅容器内网，S3/控制台只绑 127.0.0.1
```

| 容器 | 服务 | 宿主映射 | 数据卷 |
|---|---|---|---|
| zhishu-frontend | frontend | `0.0.0.0:8088 → :80`（局域网入口） | — |
| zhishu-app | app | `127.0.0.1:18080 → :8080`（仅本机调试） | — |
| zhishu-postgres | postgres | 不对外 | `zhishu_postgres-data` |
| zhishu-redis | redis | 不对外 | `zhishu_redis-data`（AOF 持久） |
| zhishu-docreader | docreader | 不对外(仅内部 expose) | — |
| zhishu-minio | minio | `127.0.0.1:9000/9001`（仅本机） | `zhishu_minio_data` |

> 卷名按 `docker compose -p zhishu` 推导，实际以 `docker volume ls` 为准。

## 二、本目录文件说明

| 文件 | 在哪运行 | 作用 |
|---|---|---|
| `docker-compose.override.yml` | 服务器 | 覆盖自建镜像 + 统一 `zhishu-*` 容器名 + redis 持久卷 |
| `gen-env.sh` | 服务器 | 基于 `.env.example` 生成生产 `.env`（随机密钥 + MinIO/LangFuse/端口覆盖） |
| `build-and-push.sh` | 开发机 | 构建 `linux/amd64` 的 app/ui 镜像并推国内云仓库 |
| `README.md` | — | 本文档 |

不改动仓库根 `docker-compose.yml` 与 `config/config.yaml`。

## 三、服务器前置（一次性）

```bash
cat /etc/redhat-release                 # 记录系统版本
docker compose version                  # 必须 ≥ v2.20；缺则补装 docker-compose-plugin
docker version --format '{{.Server.Version}}'
# 可选：若拉 Docker Hub 官方镜像慢/超时，配置加速后重启 docker
#   /etc/docker/daemon.json  ->  {"registry-mirrors":["https://docker.m.daocloud.io"]}
#   systemctl restart docker
```
放行防火墙：`8088/tcp`（访问入口）+ 22 + 宝塔面板端口即可，不需要 80/443。

## 四、开发机（Mac）：构建并推送镜像

前置：Docker 已配镜像加速；能访问国内云仓库并已 `docker login`；有 node 环境。

```bash
cd <仓库根>/WeKnora
ZHISHU_REG=<endpoint>/<命名空间> ./deploy/zhishu/build-and-push.sh
# 例：ZHISHU_REG=registry.cn-hangzhou.aliyuncs.com/zhishu
#     ZHISHU_REG=ccr.ccs.tencentcloud.com/zhishu
```
- TAG 默认当天日期（如 `20260908`），构建结束会打印。
- 若 `WITH_ANYDOC=1` 构建卡在 Rust/anydoc 下载，可去掉该 build-arg（解析仍由 docreader 承担）。
- 注意：工作区的未提交改动会被带进镜像，确认后再编。

## 五、服务器：落盘与启动

建部署目录并上传文件（`rsync` 或宝塔文件均可）：

```bash
# 在 /opt/zhishu 下应包含：
#   docker-compose.yml          (仓库根)
#   config/config.yaml          (仓库根，app 容器 bind 挂载必需)
#   .env.example                (仓库根，gen-env 的底稿)
#   docker-compose.override.yml (本目录)
#   gen-env.sh                  (本目录)
mkdir -p /opt/zhishu
# 示例(在开发机执行)：rsync -av WeKnora/docker-compose.yml WeKnora/config/config.yaml \
#   WeKnora/.env.example WeKnora/deploy/zhishu/docker-compose.override.yml \
#   WeKnora/deploy/zhishu/gen-env.sh  root@<服务器IP>:/opt/zhishu/
```

在服务器生成 `.env` 并启动：

```bash
cd /opt/zhishu
chmod +x gen-env.sh
./gen-env.sh                # 首次生成；打印管理员密码请抄走
# 若需改仓库 endpoint / 命名空间 / tag：编辑脚本顶部 ZHISHU_REG_ENDPOINT 等再重跑

docker login <endpoint>                     # 如 registry.cn-hangzhou.aliyuncs.com
docker compose -p zhishu --profile minio pull
docker compose -p zhishu --profile minio up -d
docker compose -p zhishu ps                 # 6 容器；app/postgres/minio 应为 healthy
curl -s http://127.0.0.1:8088/ | head       # 返回 HTML 即通
```

浏览器访问 `http://<服务器IP>:8088`，用 gen-env 打印的 **admin / 密码** 登录。
> 首启若 app 日志报 bucket 不存在（WeKnora 未必自动建桶），补建一次：
> ```bash
> cd /opt/zhishu && set -a && . ./.env && set +a
> docker run --rm --network WeKnora-network \
>   -e MC_HOST_zhishu="http://${MINIO_ACCESS_KEY_ID}:${MINIO_SECRET_ACCESS_KEY}@minio:9000" \
>   minio/mc:latest mb --ignore-existing "zhishu/$MINIO_BUCKET_NAME"
> ```

## 六、首次使用

1. 登录 admin → 「设置 / 系统设置」添加**在线模型供应商**：LLM、Embedding、Rerank 各配一个 OpenAI 兼容 key（服务器需能访问对应 API 域名）。
2. 建知识库 → 传 1~2 份文档，验证 解析/检索/流式问答。
3. 局域网无域名，`FRONTEND_BASE_URL`、`APP_EXTERNAL_URL` 保持留空即可（走相对路径）。

## 七、备份

宝塔计划任务（或 cron）定时执行，卷名先 `docker volume ls | grep zhishu` 核对：

```bash
# PG（容器 zhishu-postgres）
docker exec zhishu-postgres pg_dump -U zhishu -d zhishu -Fc \
  > /backup/zhishu_$(date +%F).dump

# MinIO 文件 + redis AOF
docker run --rm -v zhishu_minio_data:/data -v /backup:/backup alpine \
  tar czf /backup/zhishu_minio_$(date +%F).tar.gz -C /data .
docker run --rm -v zhishu_redis-data:/data -v /backup:/backup alpine \
  tar czf /backup/zhishu_redis_$(date +%F).tar.gz -C /data .
```

`SYSTEM_AES_KEY` 的备份与本文件同等重要：库内加密字段丢失即永久不可解。

## 八、升级 / 回滚

1. 开发机改代码 → commit → 重新跑 `build-and-push.sh`（新 TAG）。
2. 服务器改 `.env` 里的 `ZHISHU_APP_IMAGE` / `ZHISHU_UI_IMAGE` 为新 TAG（`sed` 或编辑）。
3. `cd /opt/zhishu && docker compose -p zhishu --profile minio pull && docker compose -p zhishu --profile minio up -d`。
4. 回滚：把镜像变量改回旧 TAG 再 pull + up。旧 TAG 镜像保留在仓库即回滚点。

> 注：`docker compose -p zhishu` 的 project 名固定为 `zhishu`，保证网络/卷/容器命名稳定。

## 九、故障排查速查

| 现象 | 排查 |
|---|---|
| `docker compose` 报 version 不支持 | 检查 compose ≥ v2.20 |
| 拉取官方镜像超时 | 配 `/etc/docker/daemon.json` 加速并重启 docker |
| app 反复重启 / unhealthy | `docker logs zhishu-app` 看迁移与连库日志 |
| minio 上传报错 | `docker logs zhishu-minio`；确认 `.env` 中 STORAGE_TYPE=minio 且 bucket 已建 |
| 流式问答不逐字返回 | 这是容器内 frontend→app 直连，不受宝塔影响；若仍卡检查模型 key |
| 重启后 redis 队列状态丢 | 检查 `zhishu_redis-data` 卷已挂载 |

## 十、安全提醒

- docker.sock 沙箱保持默认关闭（`WEKNORA_SANDBOX_DOCKER_ENABLED=false`），挂载等于宿主机 root。
- minio / redis / postgres 均不暴露公网；minio 控制台需要时用 `ssh -L 9001:127.0.0.1:9001 root@<IP>` 隧道访问 `http://localhost:9001`。
- `.env` 含全部密钥，权限已 `chmod 600`，切勿提交到 git。
