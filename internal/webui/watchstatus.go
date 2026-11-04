// watchstatus.go —— Plex 观看进度叠加服务：枚举 Plex 库聚合成媒体级观看计数，
// 就地注入库快照行（watched_episodes / plex_watched），供库页卡片显示「已看 N/M」。
// 未配置 Plex 或枚举失败时静默降级：不注入任何字段，库页其余功能不受影响。
package webui

import (
	"context"
	"strconv"
	"sync"
	"time"

	"panoply/internal/jsonx"
	"panoply/internal/plexserver"
	"panoply/internal/watched"
)

const (
	// watchStatusTTL 缓存有效期：过期后下次请求触发后台刷新。
	watchStatusTTL = 15 * time.Minute
	// watchStatusFirstWait 首刷同步等待上限：超过即放弃，本次响应不带观看字段。
	watchStatusFirstWait = 5 * time.Second
	// watchStatusForceWait 「刷新媒体库」按钮路径的同步刷新上限。
	watchStatusForceWait = 15 * time.Second
)

// watchAgg 单媒体（tmdb 键）的观看聚合。
type watchAgg struct {
	watchedEpisodes int  // tv：已看集数
	watched         bool // movie：任一版本看过
}

// watchStatusService 观看进度缓存（内存级；请求驱动懒刷新，无人访问零开销）。
type watchStatusService struct {
	rt *Runtime

	mu          sync.Mutex
	agg         map[string]watchAgg
	refreshedAt time.Time
	refreshing  bool
	refreshDone chan struct{} // 进行中刷新的完成信号（并发等待者共享结果）
}

// EnsureFresh 保证缓存可用：从未刷新过 → 限时同步刷新；已过期 → 后台刷新、本次沿用旧值。
func (s *watchStatusService) EnsureFresh(ctx context.Context) {
	s.mu.Lock()
	never := s.refreshedAt.IsZero()
	fresh := !never && time.Since(s.refreshedAt) < watchStatusTTL
	s.mu.Unlock()
	if fresh {
		return
	}
	if never {
		cctx, cancel := context.WithTimeout(ctx, watchStatusFirstWait)
		defer cancel()
		s.Refresh(cctx)
		return
	}
	go s.Refresh(context.Background())
}

// Refresh 同步刷新缓存；已有刷新在进行中则等待其完成（结果共享）。
// 失败不覆盖旧数据，仅记日志（Plex 暂时不可达属预期内降级）。
func (s *watchStatusService) Refresh(ctx context.Context) {
	s.mu.Lock()
	if s.refreshing {
		done := s.refreshDone
		s.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
		}
		return
	}
	s.refreshing = true
	done := make(chan struct{})
	s.refreshDone = done
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.refreshing = false
		close(done)
		s.mu.Unlock()
	}()

	agg, err := s.collect(ctx)
	if err != nil {
		s.rt.appLog("library", "watch_refresh_failed", "WARNING",
			"Plex 观看进度刷新失败，沿用上次数据："+err.Error(), nil)
		return
	}
	s.mu.Lock()
	s.agg = agg
	s.refreshedAt = time.Now()
	s.mu.Unlock()
}

// collect 枚举 Plex 库并聚合。未配置 Plex 返回空数据（非错误，不告警）。
func (s *watchStatusService) collect(ctx context.Context) (map[string]watchAgg, error) {
	client := s.rt.GetPlexClient()
	if client == nil {
		return map[string]watchAgg{}, nil
	}
	var mapper watched.IDMapper
	if st := s.rt.GetTraktSyncStore(); st != nil {
		mapper = st // 持久 ID 映射（traktsync.db），tvdb 锚翻译用；未开同步也能读
	}
	source := plexserver.NewSource(client, s.rt.buildTitleIndex(), mapper)
	states, err := source.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return aggregateWatchStates(states, mapper), nil
}

// aggregateWatchStates 把集/电影级观看状态聚合成媒体级计数（tmdb 键）。
// 键粒度已是「集」（同集多版本在 Source.ListAll 内聚合去重），tv 只需按媒体计数。
// tvdb 锚的键经 mapper 翻译，翻译不出的丢弃（library_media 以 tmdb_id 为关联键）。
func aggregateWatchStates(states map[watched.MediaKey]watched.ItemState, mapper watched.IDMapper) map[string]watchAgg {
	agg := map[string]watchAgg{}
	for key, st := range states {
		if !st.Watched {
			continue
		}
		switch key.Type {
		case "movie":
			if key.TMDBID <= 0 {
				continue
			}
			k := "movie:" + strconv.FormatInt(key.TMDBID, 10)
			a := agg[k]
			a.watched = true
			agg[k] = a
		case "tv":
			tmdbID := key.TMDBID
			if tmdbID <= 0 && key.TVDBID > 0 && mapper != nil {
				if ids := mapper.IDsOf(key); ids.TMDB > 0 {
					tmdbID = ids.TMDB
				}
			}
			if tmdbID <= 0 {
				continue
			}
			k := "tv:" + strconv.FormatInt(tmdbID, 10)
			a := agg[k]
			a.watchedEpisodes++
			agg[k] = a
		}
	}
	return agg
}

// Annotate 就地向库快照行注入观看进度：tv → watched_episodes；movie → plex_watched。
// 无数据不写键（前端按可选字段处理，卡片回到无徽标形态）。
func (s *watchStatusService) Annotate(rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	s.mu.Lock()
	agg := s.agg
	s.mu.Unlock()
	if len(agg) == 0 {
		return
	}
	for _, row := range rows {
		annotateRow(row, agg)
	}
}

// AnnotateRow 单行注入（详情端点用）。
func (s *watchStatusService) AnnotateRow(row map[string]any) {
	if row == nil {
		return
	}
	s.mu.Lock()
	agg := s.agg
	s.mu.Unlock()
	if len(agg) == 0 {
		return
	}
	annotateRow(row, agg)
}

func annotateRow(row map[string]any, agg map[string]watchAgg) {
	mediaType, _ := row["media_type"].(string)
	tmdbID := int64(jsonx.Int(row, "tmdb_id", 0))
	if tmdbID <= 0 {
		return
	}
	switch mediaType {
	case "tv":
		if a, ok := agg["tv:"+strconv.FormatInt(tmdbID, 10)]; ok {
			row["watched_episodes"] = a.watchedEpisodes
		}
	case "movie":
		if a, ok := agg["movie:"+strconv.FormatInt(tmdbID, 10)]; ok && a.watched {
			row["plex_watched"] = true
		}
	}
}
