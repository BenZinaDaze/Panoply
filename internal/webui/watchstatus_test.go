// watchstatus_test.go —— 观看进度聚合与快照注入的单测。
package webui

import (
	"testing"

	"panoply/internal/watched"
)

// fakeIDMapper 测试用 tvdb→tmdb 翻译桩。
type fakeIDMapper struct {
	tmdbByTVDB map[int64]int64
}

func (f *fakeIDMapper) ResolveKey(string, watched.MediaIDs, int, int) (watched.MediaKey, bool) {
	return watched.MediaKey{}, false
}

func (f *fakeIDMapper) IDsOf(key watched.MediaKey) watched.MediaIDs {
	if tmdb, ok := f.tmdbByTVDB[key.TVDBID]; ok {
		return watched.MediaIDs{TMDB: tmdb, TVDB: key.TVDBID}
	}
	return watched.MediaIDs{}
}

func watchedState(w bool) watched.ItemState {
	return watched.ItemState{Present: true, Watched: w}
}

func TestAggregateWatchStates(t *testing.T) {
	mapper := &fakeIDMapper{tmdbByTVDB: map[int64]int64{777: 42}}
	states := map[watched.MediaKey]watched.ItemState{
		// tv tmdb 锚：看 3 集、没看 1 集 → 计数 3
		{Type: "tv", TMDBID: 100, Season: 1, Episode: 1}: watchedState(true),
		{Type: "tv", TMDBID: 100, Season: 1, Episode: 2}: watchedState(true),
		{Type: "tv", TMDBID: 100, Season: 2, Episode: 1}: watchedState(true),
		{Type: "tv", TMDBID: 100, Season: 2, Episode: 2}: watchedState(false),
		// tv tvdb 锚：翻译成功（tvdb 777 → tmdb 42）计入；失败（tvdb 888）丢弃
		{Type: "tv", TVDBID: 777, Season: 1, Episode: 1}: watchedState(true),
		{Type: "tv", TVDBID: 888, Season: 1, Episode: 1}: watchedState(true),
		// movie：看过注入、没看跳过、缺 tmdb 锚跳过
		{Type: "movie", TMDBID: 200}:                     watchedState(true),
		{Type: "movie", TMDBID: 201}:                     watchedState(false),
		{Type: "movie", TMDBID: 0, TVDBID: 999}:          watchedState(true),
	}
	agg := aggregateWatchStates(states, mapper)
	if got := agg["tv:100"].watchedEpisodes; got != 3 {
		t.Errorf("tv:100 watchedEpisodes = %d, want 3", got)
	}
	if got := agg["tv:42"].watchedEpisodes; got != 1 {
		t.Errorf("tvdb 锚翻译后 tv:42 watchedEpisodes = %d, want 1", got)
	}
	if _, ok := agg["tv:888"]; ok {
		t.Error("tvdb 锚翻译失败应丢弃，不应出现在聚合结果")
	}
	if !agg["movie:200"].watched {
		t.Error("movie:200 应标记已看")
	}
	if _, ok := agg["movie:201"]; ok {
		t.Error("未看的电影不应进入聚合")
	}
	if _, ok := agg["movie:0"]; ok {
		t.Error("缺 tmdb 锚的电影不应进入聚合")
	}
}

func TestAggregateWatchStatesEmpty(t *testing.T) {
	if agg := aggregateWatchStates(nil, nil); len(agg) != 0 {
		t.Errorf("空输入应返回空聚合，got %d 项", len(agg))
	}
}

// newAnnotatedSvc 构造带预置缓存的 service（Annotate 不依赖 Runtime）。
func newAnnotatedSvc(agg map[string]watchAgg) *watchStatusService {
	s := &watchStatusService{}
	s.agg = agg
	return s
}

func TestAnnotate(t *testing.T) {
	svc := newAnnotatedSvc(map[string]watchAgg{
		"tv:100":     {watchedEpisodes: 3},
		"movie:200":  {watched: true},
		"movie:201":  {}, // 电影未看：不应注入
	})
	rows := []map[string]any{
		{"media_type": "tv", "tmdb_id": 100},                       // 命中 → 注入
		{"media_type": "movie", "tmdb_id": 200},                     // 命中已看 → 注入
		{"media_type": "movie", "tmdb_id": 201},                     // 命中未看 → 不注入
		{"media_type": "tv", "tmdb_id": 999},                        // 无数据 → 不注入
		{"media_type": "tv", "tmdb_id": 0},                          // 无 tmdb_id → 跳过
		{"media_type": "tv", "tmdb_id": float64(100)},               // json 解码的 float64 → 命中
		{"media_type": "unknown", "tmdb_id": 100},                   // 未知类型 → 不注入
		{},                                                          // 空行 → 不 panic
	}
	svc.Annotate(rows)
	if got := rows[0]["watched_episodes"]; got != 3 {
		t.Errorf("tv 命中行 watched_episodes = %v, want 3", got)
	}
	if got := rows[1]["plex_watched"]; got != true {
		t.Errorf("movie 已看行 plex_watched = %v, want true", got)
	}
	if _, ok := rows[2]["plex_watched"]; ok {
		t.Error("movie 未看行不应注入 plex_watched")
	}
	if _, ok := rows[3]["watched_episodes"]; ok {
		t.Error("无数据行不应注入 watched_episodes")
	}
	if _, ok := rows[4]["watched_episodes"]; ok {
		t.Error("tmdb_id=0 行应跳过")
	}
	if got := rows[5]["watched_episodes"]; got != 3 {
		t.Errorf("float64 tmdb_id 行 watched_episodes = %v, want 3", got)
	}
	if _, ok := rows[6]["watched_episodes"]; ok {
		t.Error("未知 media_type 行不应注入")
	}
	// 单行入口与 nil 防御
	svc.AnnotateRow(rows[0])
	svc.AnnotateRow(nil)
}

func TestAnnotateEmptyCacheZeroInjection(t *testing.T) {
	svc := newAnnotatedSvc(nil)
	rows := []map[string]any{{"media_type": "tv", "tmdb_id": 100}}
	svc.Annotate(rows)
	if len(rows[0]) != 2 {
		t.Errorf("空缓存不应注入任何字段，行内容 = %v", rows[0])
	}
}
