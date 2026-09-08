# 知枢（WeKnora fork）局域网 Docker 部署

面向**局域网服务器（CentOS Stream 9 + 宝塔面板，10.0.3.109）**单机部署：浏览器直连 `http://10.0.3.109:8088`，无公网域名/SSL。

- 服务：`postgres(ParadeDB PG17，兼向量库)` + `redis` + `minio` + `app` + `frontend` + `docreader`，容器名统一 **`zhishu-*`**
- 不用 LangFuse、searxng、qdrant/milvus/neo4j/doris、sandbox、mcp 等（保持 profile 不启动）
- 模型全部在线 OpenAI 兼容 API（服务器能出公网，UI 里配置）
- **持久数据全部落在 `/home`**（部署目录 `/home/zhishu`，数据 bind 到 `/home/zhishu/data/*`）
- 镜像仓库：**局域网自建私有 registry:2**（开发机 push / 服务器 pull 全走内网）

## 一、总体拓扑

```
开发机(Mac)  --push linux/amd64-->  10.0.3.109:5000 zhishu-registry(局域网私有仓库)
                                              │ pull
                                              ▼
局域网浏览器 ─ http://10.0.3.109:8088 ─▶ zhishu-frontend(nginx :80)
                                              │ 代理 /api /files /r/
                                              ▼
zhishu-app(:8080) ─▶ zhishu-postgres(5432) / zhishu-redis(6379) / zhishu-docreader(50051) / zhishu-minio(9000)
（postgres/redis/minio/app 端口全部只走内网或 127.0.0.1，不对局域网暴露）
```

| 容器 | 来源 | 宿主映射 | 持久数据 |
|---|---|---|---|
| zhishu-registry | registry:2（一次性，非 compose） | `10.0.3.109:5000` | `/home/zhishu/data/registry` |
| zhishu-frontend | 自建 zhishu-ui | `0.0.0.0:8088 → :80` | — |
| zhishu-app | 自建 zhishu-app | `127.0.0.1:18080 → :8080` | — |
| zhishu-postgres | paradedb 官方 | 不对外 | `/home/zhishu/data/postgres` |
| zhishu-redis | redis 官方（AOF） | 不对外 | `/home/zhishu/data/redis` |
| zhishu-docreader | weknora 官方 | 仅内网 expose | — |
| zhishu-minio | minio 官方 | `127.0.0.1:9000/9001` | `/home/zhishu/data/minio` |

> 数据目录全部 bind 到 `/home`，由本目录 `docker-compose.override.yml` 的卷定义完成；
> compose project 固定 `-p zhishu`，网络/容器命名稳定。

## 二、本目录文件说明

| 文件 | 在哪运行 | 作用 |
|---|---|---|
| `setup-lan-registry.sh` | 服务器(一次) | 起局域网私有仓库 zhishu-registry |
| `docker-compose.override.yml` | 服务器 | 自建镜像 + `zhishu-*` 命名 + redis 持久卷 + **数据卷 bind 到 /home** |
| `gen-env.sh` | 服务器 | 生成生产 `.env`（随机密钥 + MinIO/LangFuse/端口/镜像地址覆盖） |
| `build-and-push.sh` | 开发机 | 构建 `linux/amd64` 的 app/ui 并推局域网仓库 |
| `README.md` | — | 本文档 |

不改动仓库根 `docker-compose.yml` 与 `config/config.yaml`。

## 三、服务器前置（一次性）

```bash
cat /etc/redhat-release                 # CentOS Stream release 9
docker compose version                  # 必须 ≥ v2.20；缺则补装 docker-compose-plugin
docker version --format '{{.Server.Version}}'
# 若服务器拉 Docker Hub 官方镜像(docreader/paradedb/redis/minio)慢/超时，配加速：
#   /etc/docker/daemon.json ->  {"registry-mirrors":["https://docker.m.daocloud.io"]}
#   systemctl restart docker
```
放行防火墙：`8088/tcp`（访问入口）+ `5000/tcp`（内网拉镜像）+ 22 + 宝塔面板端口。80/443 不需要。

### 3.1 建立局域网私有镜像仓库（zhishu-registry）
```bash
bash <服务器路径>/setup-lan-registry.sh
# 数据在 /home/zhishu/data/registry
```
**关键：两端 docker daemon 都要信任这个 HTTP 仓库**（insecure-registries），否则 push/pull 报 `http: server gave HTTP response to HTTPS client`：
- 服务器：编辑 `/etc/docker/daemon.json`（与 registry-mirrors 并存）：
  ```json
  { "registry-mirrors": ["https://docker.m.daocloud.io"],
    "insecure-registries": ["10.0.3.109:5000"] }
  ```
  然后 `systemctl restart docker`。
- 开发机(Mac)：Docker Desktop → Settings → Docker Engine → 加同一键 → Apply & Restart。

自检（开发机）：`docker pull busybox && docker tag busybox 10.0.3.109:5000/zhishu/smoke:1 && docker push 10.0.3.109:5000/zhishu/smoke:1`。

> 镜像方案说明：Mac 与服务器同处 10.0.3.x 内网，自建 registry 让 app/ui 推拉全走内网，
> 最快且无云账号/公网依赖，旧 tag 留在仓库即回滚点。若将来要公网/多机，可切国内云 ACR/TCR：
> `ZHISHU_REG=registry.cn-hangzhou.aliyuncs.com/zhishu ZHISHU_REG_USER=.. ZHISHU_REG_PASS=.. ./build-and-push.sh`。
> 官方镜像（docreader/PG/redis/minio）始终由服务器直拉 Docker Hub。

### 3.2 落盘目录与数据目录
```bash
mkdir -p /home/zhishu/data/{postgres,minio,redis,files}
```
> postgres/minio 容器首启会自行 chown/初始化；如遇权限报错按 README 第九节处理。

## 四、开发机（Mac）：构建并推送镜像

前置：Docker 已配镜像加速 + 已加 `10.0.3.109:5000` 到 insecure-registries；有 node 环境。

```bash
cd <仓库根>/WeKnora
./deploy/zhishu/build-and-push.sh          # 默认推 10.0.3.109:5000/zhishu，TAG=当天日期
# TAG=20260908 ./deploy/zhishu/build-and-push.sh   # 指定版本号
```
- TAG 打印在结尾，务必与服务器 gen-env 的 ZHISHU_TAG 一致。
- 若 `WITH_ANYDOC=1` 构建卡 Rust/anydoc 下载，去掉该 build-arg（解析仍由 docreader 承担）。
- 工作区未提交改动会被带进镜像，确认后再编。

## 五、服务器：落盘文件并启动

```bash
# 在 /home/zhishu 下应包含：
#   docker-compose.yml          (仓库根)
#   config/config.yaml          (仓库根，app 容器 bind 挂载必需)
#   .env.example                (仓库根，gen-env 底稿)
#   docker-compose.override.yml / gen-env.sh / setup-lan-registry.sh (本目录)
mkdir -p /home/zhishu
# 例(开发机执行)：rsync -av WeKnora/docker-compose.yml WeKnora/config/config.yaml \
#   WeKnora/.env.example WeKnora/deploy/zhishu/docker-compose.override.yml \
#   WeKnora/deploy/zhishu/gen-env.sh WeKnora/deploy/zhishu/setup-lan-registry.sh \
#   root@10.0.3.109:/home/zhishu/
```

```bash
cd /home/zhishu
chmod +x gen-env.sh setup-lan-registry.sh
./gen-env.sh                # 首次生成 .env；打印的管理员密码请抄走
# 仓库 endpoint/命名空间/tag 若要改：编辑脚本顶部 ZHISHU_REG_* / ZHISHU_TAG 再重跑

docker compose -p zhishu --profile minio pull
docker compose -p zhishu --profile minio up -d
docker compose -p zhishu ps                 # 6 容器；app/postgres/minio healthy
curl -s http://127.0.0.1:8088/ | head       # 返回 HTML 即通
```

浏览器访问 `http://10.0.3.109:8088`，用 gen-env 打印的 **admin / 密码** 登录。
> 首启若 app 日志报 bucket 不存在（WeKnora 未必自动建桶），补建一次：
> ```bash
> cd /home/zhishu && set -a && . ./.env && set +a
> docker run --rm --network zhishu_WeKnora-network \
>   -e MC_HOST_zhishu="http://${MINIO_ACCESS_KEY_ID}:${MINIO_SECRET_ACCESS_KEY}@minio:9000" \
>   minio/mc:latest mb --ignore-existing "zhishu/$MINIO_BUCKET_NAME"
> ```

## 六、首次使用

1. 登录 admin → 「设置」添加**在线模型供应商**：LLM、Embedding、Rerank 各配一个 OpenAI 兼容 key（服务器需能访问对应 API 域名）。
2. 建知识库 → 传 1~2 份文档，验证 解析/检索/流式问答。
3. 局域网无域名，`FRONTEND_BASE_URL`、`APP_EXTERNAL_URL` 保持留空（相对路径即可）。
4. 需要管理 MinIO 时：`ssh -L 9001:127.0.0.1:9001 root@10.0.3.109` 后浏览器开 `http://localhost:9001`。

## 七、备份

宝塔计划任务（或 cron）定时执行；数据都在 `/home/zhishu/data/`，直接打包即可：

```bash
# PG 逻辑备份（容器 zhishu-postgres）
docker exec zhishu-postgres pg_dump -U zhishu -d zhishu -Fc > /home/backup/zhishu_$(date +%F).dump

# 数据目录整体打包（minio / redis / 镜像仓库索引可按需拆分）
tar czf /home/backup/zhishu_data_$(date +%F).tar.gz -C /home/zhishu/data .
```

`SYSTEM_AES_KEY` 的备份与本文件同等重要：库内加密字段丢失即永久不可解。

## 八、升级 / 回滚

1. 开发机改代码 → commit → `./deploy/zhishu/build-and-push.sh`（新 TAG）。
2. 服务器改 `.env` 的 `ZHISHU_APP_IMAGE` / `ZHISHU_UI_IMAGE` 新 TAG（`sed -i` 或编辑）。
3. `cd /home/zhishu && docker compose -p zhishu --profile minio pull && docker compose -p zhishu --profile minio up -d`。
4. 回滚：镜像变量改回旧 TAG 再 pull + up。旧 TAG 仍在局域网仓库即回滚点。

## 九、故障排查速查

| 现象 | 排查 |
|---|---|
| push/pull 报 `http: server gave HTTP response to HTTPS client` | 两端 daemon 未配 `insecure-registries: ["10.0.3.109:5000"]` |
| 服务器拉官方镜像超时 | `/etc/docker/daemon.json` 配 registry-mirrors 后重启 |
| `docker compose` 报 version 不支持 | 检查 compose ≥ v2.20 |
| app 反复重启 / unhealthy | `docker logs zhishu-app` 看迁移与连库日志 |
| postgres 数据目录权限报错 | 检查 `/home/zhishu/data/postgres` 属主/可写，必要时 `chown -R 999:999`（PG 默认 uid） |
| minio 上传/启动报权限 | `chown -R 1000:1000 /home/zhishu/data/minio`（镜像默认非 root uid） |
| bucket 不存在 | 见第五节 mc 补建命令 |
| 流式问答不逐字返回 | frontend→app 容器内直连不受影响；查模型 key / 后端日志 |

## 十、安全提醒

- docker.sock 沙箱保持默认关闭（`WEKNORA_SANDBOX_DOCKER_ENABLED=false`），挂载等于宿主机 root。
- minio/redis/postgres 均不对局域网暴露；minio S3/控制台只绑 127.0.0.1。
- zhishu-registry 无认证，firewall 只放行内网网段（10.0.3.0/24）访问 5000。
- `.env` 含全部密钥，权限已 `chmod 600`，切勿提交到 git。
