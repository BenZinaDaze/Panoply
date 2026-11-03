# AniBT Open API 资源检索参考

来源：https://wiki.anibt.net/docs/open-api （机器可读定义：https://wiki.anibt.net/openapi.yaml ，OpenAPI 3.1）  
用途：为 Panoply 后续实现 AniBT 资源检索、RSS 订阅、字幕组 slug 映射与 Bangumi ID 查询提供本地参考。更新时间：2026-11-03（对照 2026-07-01 版重构，主要差异：RSS `limit` 上限降为 100、`/api/anime/groups` 响应改为对象、发布接口强制携带种子、新增番剧翻页搜索与 Torznab 等端点）。

## 基本约定

- Base URL：`https://anibt.net`
- 公开读接口多数无需鉴权；发布类接口和私人订阅需要鉴权（字幕组 API Key、个人 RSS 令牌、登录会话三者不能互相替代）。
- 字幕组使用稳定 `slug`，不要依赖展示名。启动时建议拉取 `/api/subtitle-groups` 建立 `name -> slug` 映射。
- 番剧查询优先使用 Bangumi subject id，即 `bgmId`。自然语言番名应先通过 `/api/bgm/search` 转换；`q` 传纯数字时按 Bangumi ID 查询。
- 支持 `ETag` / `If-None-Match`；轮询 RSS 时应保存上次 `ETag`，命中后服务端返回 `304 Not Modified`（条件请求仍可能消耗限流额度）。
- JSON 成功响应有 `data`、`result` 和无包装三种形态，解析时不要假设固定包装。
- 状态码语义：
  - `308`：跳转规范 URL。bgm.tv 合并过的旧 `bgmId` 会 308 到新 ID 的同一 URL（其他参数不变），跟随即可，不必重新订阅；与默认值等价的参数写法（如显式 `limit=100`）也会 308。
  - `429`：超出每 IP 限流，或未命中缓存的回源请求超出上限（默认每客户端 8 次 / 1 秒、全站合计 60 次 / 1 秒；命中缓存不计入），通常等 1 秒即可，按 `Retry-After` 等待。
  - `503`：后端暂时不可用（非限流），搜索类返回 `SEARCH_UNAVAILABLE`，带 `Retry-After: 30` 与 `Cache-Control: no-store`，不会以空结果代替。

## 推荐检索流程

1. 调用 `GET /api/subtitle-groups`，缓存字幕组 `slug`、名称和统计信息。
2. 用户输入番名时调用 `GET /api/bgm/search?q=<keyword>&limit=<n>`，取得 `bgmId`。
3. 用 `GET /rss/anime.xml?bgmId=<bgmId>&groupSlug=<slug>` 拉取资源 RSS。
4. 轮询时带 `If-None-Match: <etag>`，避免重复下载完整 RSS。

## 端点速查

| 场景 | 方法与路径 | 说明 |
| --- | --- | --- |
| 字幕组目录 | `GET /api/subtitle-groups` | 公开列表，返回 slug、展示名、organizationId、发布数、番剧数。`status` 参数省略或 `ACTIVE`。 |
| 当前字幕组信息 | `GET /api/subtitle-groups/me` | 需鉴权。 |
| Bangumi 搜索 | `GET /api/bgm/search?q=<keyword>&limit=<n>` | 将番名映射为 `bgmId`；q 也可传纯数字 Bangumi ID。 |
| 番剧 RSS 主入口 | `GET /rss/anime.xml?<query>` | 支持番剧、字幕组、分辨率、语言等过滤。 |
| 字幕组短链 RSS | `GET /rss/group/<slug>.xml` | 等价于只按字幕组过滤；路径必须以 `.xml` 结尾。 |
| 全站磁力 RSS | `GET /rss/magnets.xml` | 全站最新番剧磁力 RSS；搜索词最多 16 正向词 + 8 排除词，超出 400。 |
| 其他类 RSS | `GET /rss/other/<category>.xml` | `manga`、`music`、`raw`、`stage`、`collection`、`movie`、`other`。 |
| 私人订阅 RSS | `GET /rss/subscriptions.xml` | 需登录认证，返回“我的订阅”。 |
| 季度番剧 | `GET /api/seasons/anime` | 支持季度、全库 `query` 或 `bgmId` 精确查询；完整翻页用 `/api/animes/search`。 |
| 番剧游标搜索 | `GET /api/animes/search` | 全库标题搜索 + `cursor` 翻页（`limit` 默认 24，1–50），返回 `page` / `isDone` / `continueCursor`，无 `ok`/`data` 包装。cursor 为不透明令牌，无效返回 422。 |
| 单番剧字幕组 | `GET /api/anime/groups?bgmId=<id>` | 查询某番有哪些字幕组发布，见下方响应结构。 |
| 种子匹配 | `GET /api/animes/torrent-match` | 按种子信息匹配番剧。 |
| 发布身份 | `GET /api/releases/publish` | 发布前核验 Key 身份与 scopes。 |
| 发布番剧资源 | `POST /api/releases/publish` | 字幕组写入端点，必须携带种子，见下文。 |
| 发布其他资源 | `POST /api/other-releases/publish` | 发布漫画、音乐、RAW、舞台剧等。 |
| 资源详情/删除 | `GET/DELETE /api/releases/{releaseId}` | 删除需 `releases:delete`。 |
| 外站同步状态 | `GET /api/releases/{releaseId}/sync-status` | 需 `releases:sync:read`，查询萌番组、蜜柑、花园、末日站、ACG.rip 与 Nyaa 的同步结果。 |
| 种子下载 | `GET /api/torrent/{releaseId}.torrent` | RSS enclosure 即指向此地址。 |
| Torznab | `GET /torznab/api` | Sonarr / Radarr / Prowlarr 索引器入口；RSS 无分页，Torznab 使用有界 `offset`。 |

## `/rss/anime.xml` 查询参数

至少提供 `bgmId` 或 `groupSlug` 之一，否则请求会被拒绝。

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `bgmId` | integer | Bangumi.tv subject id。别名：`bangumiId`。 |
| `groupSlug` | string | 字幕组 slug。别名：`subgroupSlug`、`subgroupid`。 |
| `resolution` | enum[] | `4K`、`2160p`、`1080p`、`720p`、`480p`、`360p`。 |
| `language` | enum[] | `CHS`、`CHT`、`JP`、`EN`。 |
| `format` | enum[] | `MKV`、`MP4`、`AVI`、`WEBM`。 |
| `subtitle` | enum[] | `EXTERNAL`、`INTERNAL`、`EMBEDDED`、`NONE`。 |
| `episode` | integer | 数字集数，精确匹配。 |
| `episodeKey` | string | 显示集数键，如 `01`、`00-01`、`BATCH`。 |
| `limit` | integer | 默认 100，**上限 100**（2026-10 前为 200），省略即拉满。**不要显式传 `limit=100`，会 308 到规范 URL**。 |

重复参数可用两种写法：

```text
?resolution=1080p&resolution=720p
?resolution=1080p,720p
```

每个 `<item>` 包含：`<title>`、`<link>`（`/release/<id>` 页面）、`<guid>`、`<pubDate>`（RFC 822，`+0800`）、`<enclosure>`（指向 `/api/torrent/<releaseId>.torrent`）、`<torrent>` 容器内的 `<infohash>`，以及 `<anibt:*>` 扩展标签（`torrentUrl`、`bgmId`、`episode`、`resolution`、`language`、`fileSize` 等）。目标番剧暂时无法解析时返回 `503`（`Retry-After: 30`），不返回 `404`。

## 常用示例

```bash
# 搜索 bgmId
curl -s -G 'https://anibt.net/api/bgm/search' \
  --data-urlencode 'q=葬送的芙莉莲' \
  -d 'limit=3'

# 单番剧 + 单字幕组
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=kirara-fantasia'

# 跨字幕组聚合一个番剧
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&resolution=1080p&language=CHS'

# 字幕组全部发布
curl 'https://anibt.net/rss/anime.xml?groupSlug=kirara-fantasia&limit=50'

# 字幕组短链
curl 'https://anibt.net/rss/group/kirara-fantasia.xml'

# 合集或特典
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=kitauji-sub&episodeKey=BATCH'

# ETag 复用
ETAG=$(curl -sI 'https://anibt.net/rss/anime.xml?bgmId=543360' | awk -F': ' 'tolower($1)=="etag"{print $2}' | tr -d '\r')
curl -sI -H "If-None-Match: $ETAG" 'https://anibt.net/rss/anime.xml?bgmId=543360'
```

## JSON 读接口

### `GET /api/subtitle-groups`

返回所有可见字幕组的精简信息。关键字段包括：

- `slug`：URL 安全稳定标识，用于 `groupSlug`。
- `name`：展示名，可能变更。
- `organizationId`：内部组织 ID（不属于公开契约，仅展示用）。
- `totalReleases` / `totalAnimes`：发布总数 / 覆盖番剧数。

缓存：`s-maxage=300`，`stale-while-revalidate=86400`，支持 ETag。

### `GET /api/bgm/search`

参数：

- `q`：必填，1–100 字符；中、英、日或罗马音关键词，纯数字按 Bangumi ID 查询。`q`、`limit` 均只允许出现一次，无效查询返回 422。
- `limit`：默认 5，上限 25。

结果按匹配程度分级排序：完全相等 → 前缀 → 子串 → 多词（2–16 词）→ 字序（2–4 个中日文字符，可“不连续”）→ 相似度；同级按热度排序。匹配前统一大小写、全半角，片假名折叠为平假名，繁体/新字体折叠为简体，忽略空格和标点。1–2 字符查询不做子串/相似度匹配；短单词字母换位（如 "freiren"）可能找不到。站内未收录的作品搜不到，不保证第一条就是目标作品。

响应里 `data[].bgmId` 是后续 RSS 和发布接口应使用的权威番剧 ID。生产代码应本地缓存 `bgmId`，不要每次拉 RSS 前都搜索。

### `GET /api/seasons/anime`

查询季度番剧、全库标题搜索或精确 ID 查询。`bgmId` 优先于 `season`，不能与 `query` 同时使用。支持：

- 无参数：默认当前季度。
- `season=2026春` 或 `season=2026-SPRING`；`query`（别名 `q`）全库搜索，最长 120 字符，最多 50 条。
- `bgmId=543360`：精确返回单个番剧。

季度浏览最多 200 条，超过时按人气档位、再按评分取前 200；完整翻页用 `/api/animes/search`。已合并的旧 bgmId 返回合并后番剧：`scope.bgmId` 保持请求值，条目的 `bgmId` 是合并后的 ID。管理员隐藏或 bgm.tv 标记为 NSFW 的番剧不出现在任何结果里。

### `GET /api/anime/groups?bgmId=<id>`

查询某番剧下的发布字幕组和最近资源。**响应 `data` 为对象**（2026-10 前为顶层数组），结构：

```json
{
  "ok": true, "ts": 1790325498619,
  "data": {
    "bgmId": 543360,
    "animeId": "nh73r0ernee7s0afv9s82f763983gcz7",
    "title": { "primary": "...", "chinese": "...", "japanese": "..." },
    "groups": [
      {
        "groupId": "org_q0kdyFpr9SHP",
        "slug": "kitauji-sub",
        "name": "北宇治字幕组",
        "status": "ACTIVE",
        "lastUpdatedAt": 1784573904863,
        "totalReleases": 33,
        "publishedEpisodes": [1, 2],
        "items": [ { "releaseId": "...", "title": "...", "magnet": "...", "size": 0, "episodeKey": "12", "resolution": "1080p", "publishedAt": 1784573904863 } ]
      }
    ],
    "total": 8
  }
}
```

每组返回最近 30 条资源（`items`）；`magnet`、`size`、`episodeKey`、`resolution` 等可为 null。未找到番剧、番剧被管理员隐藏或被 bgm.tv 标记为 NSFW 时返回 `404`；暂时无法提供完整结果时返回 `503 SEARCH_UNAVAILABLE`（带 `Retry-After`）。已合并的旧 bgmId 返回合并后番剧的字幕组，`data.bgmId` 保持请求值。

## 其他 RSS

- `/rss/group/<slug>.xml`：等价于 `/rss/anime.xml?groupSlug=<slug>`，支持 `limit`，不支持 Tag 过滤。
- `/rss/magnets.xml`：全站最新番剧磁力，支持 `limit`、关键词和常见 Tag 过滤。搜索词规则：拉丁词整词匹配（`HEV` 找不到 `HEVC`）、中日文按连续字符、支持 `-词` 排除与引号短语（全角 `－`/`＂` 同义），最多 16 正向词 + 8 排除词，超出返回 400；`/magnets` 入口无结果时会放宽一次（词前缀 + 番剧标题相似度两级回退）。
- `/rss/other/<category>.xml`：其他类资源，`category` 为 `manga`、`music`、`raw`、`stage`、`collection`、`movie`、`other`，支持 `q`、`limit`、`sortField`（`publishedAt` / `fileSize`）等；无 `<anibt:bgmId>`。
- `/rss/subscriptions.xml`：私人订阅 RSS，`Cache-Control: private`，需 Bearer 或 Cookie 认证，支持 ETag。

## 发布接口概要

发布接口用于字幕组自动上传资源，不是资源检索主流程，但可用于理解 AniBT 数据来源。**自 2026-09-10 起发布必须携带种子**：multipart 上传 `.torrent` 文件，或 JSON 提供属于当前字幕组的 `torrentStorageId`（JSON 还必须提供 `magnetBase64`）；纯磁力发布返回 `422 TORRENT_REQUIRED`。

### `POST /api/releases/publish`

鉴权：

```text
Authorization: Bearer <YOUR_API_KEY>
```

- `animeIdType` + `animeId`：**必填**，不按标题匹配番剧。`animeIdType` 为 `bgm` / `mal` / `anidb` / `anilist`；bgm 的 animeId 也接受 bgm.tv 链接、全角数字、前导零。已合并的旧 BGM ID 发布到合并后的番剧。
- `Idempotency-Key`：从首次受理起绑定完整请求，同键改参返回 `409 CONFLICT`；503 重试必须复用原 Key。
- `title`：multipart 省略时用种子内部名；JSON 必填。
- 集数、画质、语言、字幕、格式、版本未显式提供时先从标题、再从种子内部名推断；无法推断集数时需传 `episode` 或 `episodeKey`。
- `preview=true`：预览模式，不正式发布，回执 10 分钟后过期。
- 请求上限：JSON 1 MiB，multipart 8 MiB，单个 `.torrent` 最多 5 MiB；`notes` 最多 50000 字符。

### `POST /api/other-releases/publish`

用于非番剧资源。必填：`category`（`manga`、`music`、`raw`、`stage`、`collection`、`movie`、`other`）、`title`、以及 `torrent` / `torrentStorageId` / `magnetLink` / `magnetBase64` 至少其一（同样受“必须携带种子”约束，纯磁力会被拒）。

常见错误：`400` 参数错误，`401` 未传 Bearer，`403` API Key 无效，`409` Idempotency-Key 冲突，`413` 请求体过大，`422` 校验失败（含 `TORRENT_REQUIRED`），`429` 写入频率过高，`503` 发布暂不可用。

## 缓存、限流与轮询建议

| 端点 | 默认 limit | 上限 | CDN TTL / SWR | 默认限流 |
| --- | ---: | ---: | --- | --- |
| `/rss/anime.xml` | 100 | 100 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/rss/group/<slug>.xml` | 100 | 100 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/rss/magnets.xml` | 100 | 100 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/rss/other/<category>.xml` | 100 | 100 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/api/bgm/search` | 5 | 25 | 300s / 300s | 60 次 / 60 秒 / IP |
| `/api/subtitle-groups` | 全量 | 全量 | 300s / 86400s | 60 次 / 60 秒 / IP |
| `/api/seasons/anime` | - | 200（季度） | 600s / 86400s | 30 次 / 60 秒 / IP（与 `/api/anime/groups` 共用） |
| `/api/anime/groups` | - | - | 300s / 3600s | 30 次 / 60 秒 / IP（与 `/api/seasons/anime` 共用） |

客户端建议：

- RSS 轮询间隔建议 5-10 分钟，不要低于 1 分钟。
- 每次保存响应 `ETag`，下次带 `If-None-Match`。
- `bgmId` 和 `slug` 应长期缓存；字幕组目录可以每天或启动时刷新。
- 对多字幕组聚合，优先使用一个 RSS URL 的逗号参数或重复参数，不要为每个字幕组发独立请求。
- 遇 `429` 按 `Retry-After` 等待（回源上限通常只需 1 秒）；遇 `503` 等待约 30 秒后重试，不要以空结果当作真实数据。

## 接入注意事项

- `slug` 优先于字幕组 `name`；`name` 可用于展示，不应用作查询键。
- `bgmId` 优先于自然语言番名；同名、多季、剧场版容易撞车。
- 路径和查询参数是公开契约；内部主键如 `organizationId` 不属于公开契约。
- 如果同时发送 `If-None-Match` 和 `If-Modified-Since`，服务端按 HTTP 规范优先使用 ETag。
- Panoply 侧对应实现见 `internal/scraper/anibt.go`：`SearchMedia` 用 `/api/bgm/search`，字幕组列表用 `/api/anime/groups`（解析兼容新旧两种结构），资源用 `/rss/anime.xml`。
