# 回归语料

设计/修改识别规则后，把候选规则套在本清单上跑一遍，结果必须与不带规则时逐字段一致（或差异明确是修复）。清单与 `internal/mediaparser/metainfo_test.go` 的 `TestMetaInfoAnimeSxxExx` 同源，随版本演进同步更新。

用法：临时测试里循环 `MetaInfo(title, "", true, rules, nil)`，对比 `rules=nil` 的基线输出。

## anitomy 路径（`[组]` 前缀）

| 标题 | 预期 CnName | 预期 EnName | 季 | 集 |
|---|---|---|---|---|
| `[Nix-Raws] 装备我最强 / 裝備我最強 / テムパル～アイテムの力～ / Tempal Item no Chikara S01E06 [CR WEB-DL 1080p AVC AAC][简繁内封]` | 装备我最强 | Tempal Item No Chikara | 1 | 6 |
| `[Group] 装备我最强 S01E06 [1080p]` | 装备我最强 | | 1 | 6 |
| `[Group] テムパル～アイテムの力～ S01E06 [CR WEB-DL 1080p]` | テムパル～アイテムの力～ | | 1 | 6 |
| `[Neko] 葬送的芙莉莲 / Sousou no Frieren S01E28 [CR WEB-DL 1080p]` | 葬送的芙莉莲 | Sousou No Frieren | 1 | 28 |
| `[Group] Title S02E13v2 [1080p][HEVC]` | | Title | 2 | 13 |
| `[Group] Title S1E6 [720p]` | | Title | 1 | 6 |
| `[Group] Title S01 E06 [1080p]` | | Title | 1 | 6 |
| `[Group] Title [S01E06][1080p]` | | Title | 1 | 6 |
| `[SubsPlease] Frieren - 12 (1080p) [04B6C9EE].mkv` | | Frieren | – | 12 |
| `[LoliHouse] 我独自升级 / Solo Leveling - 05 [WebRip 1080p HEVC-10bit][简繁字幕]` | 我独自升级 | Solo Leveling | – | 5 |
| `[LoliHouse] 东岛丹三郎想成为假面骑士 / Toujima Tanzaburou wa Kamen Rider ni Naritai - 23 [WebRip 1080p HEVC-10bit AAC][简繁内封字幕]` | 东岛丹三郎想成为假面骑士 | Toujima Tanzaburou Wa Kamen Rider Ni Naritai | – | 23 |
| `[桜都字幕组] 关于邻家的天使大人 第二季 [05][1080p][简繁内封].mkv` | 关于邻家的天使大人 | | 2 | 5 |
| `【FSD】东岛丹三郎想成为假面骑士[02][我是塔克尔].mp4` | 东岛丹三郎想成为假面骑士 | | 1 | 2 |
| `[Group] 标题 第02话 [1080p]` | 标题 | | – | 2 |
| `[Group] 标题 第二季 S02E05 [1080p]` | 标题 | | 2 | 5 |

## metavideo 路径（无组名前缀，规则误伤重灾区）

| 标题 | 预期 CnName | 预期 EnName | 季 | 集 |
|---|---|---|---|---|
| `Show.Name.S01E06.720p.mkv` | | Show Name | 1 | 6 |
| `The.Last.of.Us.S01E09.2160p.WEB-DL.DDP5.1.Atmos.H.265.mkv` | | The Last Of Us | 1 | 9 |
| `西部世界 第二季 S02E03 1080p.mkv` | 西部世界 | | 2 | –（历史行为，勿因规则改变） |

## 已知边界（当前不支持的形态，勿当作 bug 修规则）

- `S01E06E07` / `S01E06-07` 多集连写：`6E` 间无词边界，`S(\d{2})E(\d{2})\b` 不匹配
- `[05/06]` 进度标记不识别为集号（只留末段 `06` 且不解析）
- 文件名含 `/` 的路径形标题（真实文件系统不可能出现，无需支持）
