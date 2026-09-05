# Token 统计与计费设计

> 状态:设计稿(已评审通过口径,未实施)
> 日期:2026-09-05

## 一、目标与边界

1. **只计数,不计钱流**:无扣费、无余额、无限额拦截、无充值/支付/发票;金额仅为"单价 × 用量"的展示折算。
2. **不依赖 Langfuse**:自有采集链路;Langfuse 开启与否与本方案互不影响(两套并行,口径互相独立,估算规则保持一致以便对照)。
3. **消耗即计数**:请求失败/中断,只要已产生 token 消耗就入账;完全失败(连接不通、零消耗)不记录。
4. 模型全部由管理后台统一配置,计量归属链路固定(tenant / user / 模型均已知),不存在自带 Key 的归属歧义。

## 二、总体架构

```
模型调用层装饰器(新增 meter wrapper,与现有 langfuse wrapper 同位安装)
  chat / embedding / rerank / vlm / asr 五个包
        │  内存队列 → 批量落库(异步,不阻塞请求)
        ▼
model_usage_records(统一用量流水,append-only)
        │                          │
        ▼                          ▼
model_prices(后台配单价)    usage_daily_rollups(可选二期,预聚合)
        └───────────┬─────────────┘
                    ▼
     报表 API + 前端(个人设置 / 空间管理员 / 管理后台 三级入口)
```

## 三、采集层

### 3.1 埋点位置

每个模型包已有成熟装饰器模式:工厂函数按 `concurrency → debug → langfuse` 链式包装
(见 `internal/models/embedding/embedder.go` NewEmbedder),chat / vlm / rerank / asr
各有同款 `langfuse_wrapper.go`。meter wrapper 与之同位、**无条件安装**(不受 Langfuse 开关影响):

| 包 | 计量内容 |
|---|---|
| `internal/models/chat` | 输入 / 输出 / 缓存 token(provider 精确返回,`TokenUsage` 已有) |
| `internal/models/embedding` | 输入 token(provider 返回;不回则 runes/4 估算,extra 标 `approx:true`) |
| `internal/models/rerank` | query + 文档 token 估算 + 文档数 |
| `internal/models/vlm` | 输入 token + 图片张数 / 视频帧数 |
| `internal/models/asr` | 音频时长(秒)+ 输出 token |

### 3.2 归属信息来源(现成能力,走 ctx)

- `types.TenantIDContextKey` / `types.UserIDContextKey`:auth_context 中间件已注入并贯穿服务层;
- `types.LLMCallPurposeContextKey`(现成):已区分 `agent_round` / `document_summary` /
  `query_rewrite` / `question_generation` / `entity_extraction` 等场景,chat 侧"业务场景"
  维度零成本获得;
- embedding / rerank / vlm / asr 补少量同类注入,标注 `knowledge_ingest` / `retrieval_rerank` /
  `chat_attachment` 等场景;
- 后台任务(asynq)ctx 已注入 TenantID(handler/system.go 现有模式);知识处理任务再补发起者
  user_id(取 `knowledge.created_by`),使个人维度覆盖"我上传文档的处理消耗"。

### 3.3 失败 / 中断计量规则

- provider 返回错误但带部分 usage → 记录,`status=failed`;
- 流式中断 → prompt 按实际请求估算,completion 按已接收内容估算,`status=interrupted`;
- 零消耗失败 → 不记录;
- 估算规则与 Langfuse wrapper 现有 `approxEmbeddingUsage`(runes/4)一致。

### 3.4 写入机制

进程内带缓冲队列 + 批量 INSERT(如 200 条或 5 秒,参考 langfuse Manager 的
FlushAt / FlushInterval 模式)。落库失败仅告警不重试——纯计数场景可容忍小概率丢失,
不要求强一致。后续如需更强可靠可改走 asynq 异步任务。

## 四、数据模型

### 4.1 `model_usage_records` 用量流水(append-only)

| 列 | 说明 |
|---|---|
| tenant_id / user_id | 空间维度必填;user_id 可空 = 系统后台任务 |
| model_id / model_name / provider | 模型维度 |
| category | 计费大类:chat / embedding / rerank / vlm / asr |
| purpose | 业务场景(LLMCallPurpose) |
| status | success / failed / interrupted |
| input_tokens / output_tokens / cached_tokens | |
| extra jsonb | 图片张数、音频秒数、rerank 文档数、页数、`approx:true` 估算标记 |
| ref_type / ref_id | 关联 session / message / knowledge |
| created_at | |

索引:`(tenant_id, created_at)`、`(user_id, created_at)`、`(created_at)`。
保留策略仿审计日志(`WEKNORA_AUDIT_RETENTION_DAYS` 先例),默认建议 ≥180 天
(rollup 上线后可缩短明细保留)。

### 4.2 `model_prices` 单价配置(管理后台维护)

| 列 | 说明 |
|---|---|
| model_id | 关联 models 表 |
| price_input_per_m / price_output_per_m / price_cached_per_m | 对话模型,元 / 百万 token |
| price_per_m | 嵌入 / 排序模型,元 / 百万 token |
| price_per_image / price_per_audio_min / price_per_video_min | 多媒体按次 / 时长 |
| currency | 展示用,默认 CNY |
| effective_from | 报表按事件时间匹配价格,改价不追溯历史 |
| enabled | 未配价模型按 0 元展示 |

独立表而非给 models 表加列:不动现有迁移面,且能容纳多媒体等非 token 计价项。

### 4.3 `usage_daily_rollups`(可选二期)

按 `(tenant_id, user_id, model_id, category, day)` 预聚合。明细量大后再上;
一期对流水直接 GROUP BY,有索引中规模够用。

## 五、报表 API 与三级前端入口

### 5.1 API

| 端点 | 面向 | 数据范围 |
|---|---|---|
| `GET /api/v1/me/usage/summary` | 所有登录用户 | 当前空间内本人用量 |
| `GET /api/v1/tenant/usage/summary?scope=me\|tenant` | 空间管理员(角色校验) | 本人 / 全空间 |
| `GET /api/v1/admin/usage/summary?level=platform\|tenant\|user&tenant_id=&user_id=&from=&to=&group_by=model\|category\|day` | 管理后台 | 平台总览 / 按空间 / 按个人(跨空间) |
| `GET /api/v1/admin/usage/records?...` | 管理后台 | 明细分页,过滤同上 |

统一输出:token 数(输入 / 输出 / 缓存分列)+ 多媒体量(张 / 分钟)+
按 `effective_from` 匹配单价折算的金额。

### 5.2 前端三级入口

| 入口 | 位置 | 面向谁 | 内容 |
|---|---|---|---|
| 个人 AI 使用统计 | 个人设置:`frontend/src/views/settings/` 新增面板挂进 Settings.vue | 所有用户 | 本人在当前空间的用量(按计费项 / 模型 / 时间) |
| 空间用量统计 | 个人设置中的空间管理区:与 TenantInfo / TenantMembers 同级的新面板,角色门槛同空间管理员 | 空间管理员 | 两个 tab:我的用量 / 全空间用量(含成员排行下钻) |
| 总看板 + 空间 + 个人统计 | 管理后台:`frontend/src/views/system/` 新增 UsageDashboardPanel 挂进 SystemConsole.vue | 平台管理员 | 三个 tab:总看板 / 按空间(下钻空间内成员)/ 按个人(跨空间);另加 BillingPricesPanel 单价配置 |

权限矩阵:

| 数据范围 | 普通用户 | 空间管理员 | 平台管理员 |
|---|---|---|---|
| 本人(当前空间) | ✅ | ✅ | ✅ |
| 全空间 | ❌ | ✅ | ✅ |
| 平台总览 / 任意空间 / 任意个人 | ❌ | ❌ | ✅ |

### 5.3 计价配置界面

管理后台 ModelsPanel 列出的模型可配价(新增 BillingPricesPanel):
按模型类型(chat / embedding / rerank / vlm / asr)展示对应计价字段,
对话模型输入 / 输出 / 缓存分开配,多媒体模型可选按 token 或按张 / 分钟口径。

## 六、已定口径

1. 嵌入 / 排序 token:provider 精确值优先,缺失时 runes/4 估算并标 `approx:true`。
2. 多媒体计价单位:表结构 token 与按张 / 时长都留,后台按模型选口径。
3. 知识处理个人归属:后台任务记录发起者 user_id(取 `knowledge.created_by`),
   报表口径覆盖完整;无法确定发起者的系统任务 user_id 为空,只归空间。
4. 流水保留期:默认 180 天,rollup 上线后可缩短。

## 七、实施拆分(每阶段独立可验收)

| 阶段 | 内容 | 产出 |
|---|---|---|
| 一 计量 | 两张表迁移 + 五个 meter wrapper + 批量落库 + ctx 场景补注入 | 所有口径可查库验证 |
| 二 计价 | model_prices CRUD + 四组报表 API(含金额折算与权限校验) | 后台可配价、可出数 |
| 三 展示 | 三个前端入口(个人面板 / 空间面板 / 管理后台看板)+ 保留清理(+ rollup 如需) | 完整可用 |

后端为主,不动核心架构;chat 侧因 `TokenUsage` + `LLMCallPurpose` 现成,
工作量重心在 embedding / rerank / vlm / asr 补埋点与报表聚合。

## 八、明确不做

扣费 / 余额 / 限额拦截、充值 / 支付 / 发票、账单出账流程;对 Langfuse 的任何依赖。
