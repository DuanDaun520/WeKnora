# OpenSandbox 沙箱后端

面向部署方与要改这块代码的人。本文说明 `opensandbox` 后端是什么形态、怎么配、边界在哪。
协议层的整体立场见 [沙箱协议接入说明](./sandbox-protocol.md)，CubeSandbox / E2B 的集群与
模板见 [沙箱集群与标准模板](./sandbox-cluster.md)。

## 结论先说

- OpenSandbox（[alibaba/OpenSandbox](https://github.com/alibaba/OpenSandbox)）是阿里开源的沙箱
  服务端，**不说 E2B 协议**：控制面是自己的 REST API（`/v1` 前缀、`OPEN-SANDBOX-API-KEY`
  头），数据面是注入到每个沙箱里的 execd 常驻进程（端口 44772，NDJSON 流式命令执行）。
  因此它是 WeKnora 的**第四个一等适配器**（`internal/sandbox/opensandbox_remote_client.go`），
  不是 `e2b` 配置的又一种指向。
- 数据面单源：execd 流量始终经生命周期服务端反代（`GetEndpoint(id, 44772, useServerProxy=true)`），
  控制面与数据面同源。**不需要泛域名、不需要额外的 ingress 组件**——这正是它与
  Cube / E2B 部署形态上最大的区别。
- 适合已经在跑 OpenSandbox 集群的团队（K8s runtime 或 Docker runtime 都行）。从零开始且
  只要容器隔离的话，先读 [sandbox-protocol.md](./sandbox-protocol.md) 的选型表。

## 实现要点

实现走官方 Go SDK 的**低层客户端**（`LifecycleClient` + `ExecdClient`，注入 WeKnora 的
共享守卫传输池），不用 SDK 高层封装：后者自建每沙箱连接，且其执行累加器会**丢弃退出码**。
适配器自己累加 SSE/NDJSON 事件；流结束仍拿不到退出码时，补一次 `GET /command/status/{id}`
兜底（最坏退化为 0/1，不劣于官方 SDK）。

| 契约方法 | OpenSandbox 实现 |
| --- | --- |
| `Health` | `GET /v1/sandboxes?PageSize=1`（一次调用同时验证可达性与 API key；协议无 /health） |
| `Create` | `POST /v1/sandboxes`，metadata 落成服务端可过滤标签；随后轮询 `GET /v1/sandboxes/{id}` 至 Running（≤60s）并探活 execd |
| `Connect` | `GET /v1/sandboxes/{id}`；paused 则 Resume；随后尽力 `RenewExpiration`（见下） |
| `Get` / `List` | `GET /v1/sandboxes[/{id}]`，List 走服务端 metadata 过滤 + 客户端复核，分页上限 100 页 |
| `Delete` | `DELETE /v1/sandboxes/{id}` |
| `Exec` | `POST {execd}/command`（NDJSON 流；stdout/stderr 分离），超时毫秒 + ctx 双保险 |
| `WriteFile` / `ReadFile` | execd 的 `POST /files:upload`（multipart）/ `GET /files` |
| `ListDir` / `Stat` / `MakeDir` / `Remove` | execd 的 directories / files:info 接口；MakeDir 幂等（`CreateDirectory` 即 mkdir -p）；Remove 按类型分派 DeleteDirectory（递归）/ DeleteFiles（仅文件） |
| 快照三件套 | `POST /v1/snapshots` + 轮询 Ready；删除 404 视为成功（幂等）；快照 ID 可直接当 template ID 创建沙箱 |

几个不显然但要紧的决定：

**超时是服务端强制的绝对到期时间。** 与 Cube / E2B 的「空闲 N 秒回收」不同，OpenSandbox
沙箱带 `expiresAt`：到期即销毁，跟是否活跃无关。适配器的对应策略是——创建时把配置的 TTL
作为初始到期时间（夹取 ≥60 秒，服务端下限）；**每次 `Connect`（会话继续）时尽力
`RenewExpiration(now+TTL)`**，失败只记日志。效果上 TTL 近似「会话最长静默时间」：只要会话
还在被使用，沙箱就一直续着；连续静默一个 TTL 后沙箱被服务端回收，下一次消息走
session_lifecycle 既有的重绑路径重建。会话创建请求固定 `RemoteOnTimeoutKill`。

**execd 由服务端注入，标准镜像开箱可用。** 服务端配置里的 `runtime.execd_image` 会在沙箱
启动时由 execd-installer（K8s initContainer / Docker runtime 等价机制）把 execd 二进制拷进
沙箱的 `/opt/opensandbox`——**镜像里不需要预装 execd**。因此默认模板就是标准镜像
`wechatopenai/weknora-sandbox:main`（uid 1000 的 `user` 账号 + `/workspace/{input,output}`），
与 Docker 后端的镜像要求一致。

**执行账号映射。** `User="user"` → uid/gid 1000，`"root"` → 0/0，其它值省略（由 execd 默认）。
协议没有 stdin：`RemoteExecRequest.Stdin` 通过与 Cube 相同的 heredoc 包装传入。

**网络策略与超时动作是硬边界。** OpenSandbox 创建沙箱没有网络策略参数，适配器对非零
`RemoteNetworkPolicy` 直接返回 Unsupported，对非 kill 的超时动作返回 InvalidRequest——宁可
配置时报错，也不静默降级。

## 配置

在「设置 → 沙箱后端」中新建配置并选择 OpenSandbox：

| 字段 | 说明 |
| --- | --- |
| API 端点 | 必填。生命周期服务端地址，**必须以 `/v1` 结尾**（如 `http://10.0.0.5:8080/v1`）——SDK 只做路径拼接，漏掉前缀所有请求都会 404。私网地址要打开「允许访问私网集群地址」 |
| API Key | 必填、加密存储。即服务端配置的 `api_key`；服务端未开启鉴权时也须填一个任意非空值（深检查的认证探测需要一个真实请求） |
| 模板 | 必填。含 `:` 或 `/` 按**镜像 URI** 创建，否则按**快照 ID** 创建（装过 skill 的空间自动落进第二种） |
| HTTP 超时 | 调生命周期 REST 接口的等待上限。留空 30 秒 |
| 沙箱 TTL | 见上文续期语义。留空 1800 秒；须不大于服务端 `max_sandbox_timeout_seconds`，否则创建时报错 |

服务端侧的关键配置（`~/.sandbox.toml` 或等价位置）：`api_key`（鉴权密钥）、
`max_sandbox_timeout_seconds`（沙箱到期上限；**删掉该键表示不设上限**，设了则必须 ≥60）、
`runtime.execd_image`（execd 注入镜像，默认值通常不用动）。部署方式参考 OpenSandbox
官方文档（`uvx opensandbox-server` 单进程起步，K8s 有 Helm / manifest）。

模板目录是极简形态（与 Docker 后端同款）：`ListTemplates` 返回唯一的标准条目——配置里的
镜像（默认标准镜像）或快照 ID，状态恒为 ready；「重建标准模板」在快照链路里由重新装
skill 触发，不在模板卡片上。

## 快照与技能镜像

「空间级管理沙箱装 skill → 打快照 → 会话从快照起沙箱 → 增量出下一版」已接入：
适配器实现 `RemoteSnapshotManager`，`CreateSnapshot` 会**等到 Ready** 才返回（K8s 上
快照就绪最长 15 分钟，安装器的进度条覆盖这段）；快照 ID 直接进 `CreateSandboxRequest`。
安装器写 `/opt/weknora/tenant/skills` 的那条链路与 Cube / E2B / Docker 共用。

注意：快照语义在 **K8s runtime 上已验证**；**Docker runtime 未验证**。失败会以安装报错的
形式显式出现，不会静默。

## 边界

这些是 OpenSandbox 当前映射不了的，写在这里以免被当成 bug：

- **卷挂载**：`SupportsVolumes=false`。SDK 的 Volume 是 K8s 专属形态，与租户级共享卷对不上。
- **域名级出网策略**：创建沙箱无网络策略参数，非零策略直接报 Unsupported（见上）。
- **stdin**：协议层面命令执行没有 stdin 通道，heredoc 包装是应用层模拟；极端大的 stdin
  会先落到沙箱文件系统。
- **内存态快照**：快照只保存文件系统（与 Docker commit 同级），E2B 那种内存态 pause/snapshot
  没有。

## 测试

单元测试用一个内存版双平面服务端（生命周期 + execd 同源 httptest）驱动适配器：

```bash
go test ./internal/sandbox -run 'OpenSandbox' -count=1
```

集成测试打真实服务端（凭据经环境变量传入，不入库）：

```bash
OPENSANDBOX_API_URL=http://host:8080/v1 \
OPENSANDBOX_API_KEY=<key> \
OPENSANDBOX_IMAGE=wechatopenai/weknora-sandbox:main \
go test -tags=opensandbox_integration ./internal/sandbox \
  -run '^TestOpenSandboxIntegration' -count=1 -v -timeout=15m
```

快照往返默认跳过，加 `OPENSANDBOX_SNAPSHOTS=1` 启用。
