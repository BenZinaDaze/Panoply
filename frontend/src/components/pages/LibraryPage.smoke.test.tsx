// LibraryPage.smoke.test.tsx —— 媒体库页冒烟测试：
// 空库快照下页面正常渲染（标题/筛选可达），为 library_media 数据层改造护航；
// 观看进度徽标依赖后端注入的 watched_episodes / plex_watched（未配置 Plex 时缺席）。
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { http, HttpResponse } from 'msw'
import LibraryPage from '@/components/pages/LibraryPage'

describe('LibraryPage 冒烟', () => {
  it('空库快照下正常呈现页头与筛选', async () => {
    render(
      <MemoryRouter>
        <LibraryPage filter="movies" />
      </MemoryRouter>,
    )
    expect(await screen.findByText('电影')).toBeInTheDocument()
  })

  it('剧集卡片按 Plex 观看进度显示「已看 N/M」「已看完」徽标', async () => {
    const { server } = await import('@/test/mocks/server')
    server.use(
      http.get('/api/library', () =>
        HttpResponse.json({
          movies: [],
          tv_shows: [
            { tmdb_id: 1, title: '部分看完的剧', original_title: '', year: '2024', media_type: 'tv', overview: '', rating: 0, total_episodes: 12, in_library_episodes: 12, watched_episodes: 3 },
            { tmdb_id: 2, title: '全部看完的剧', original_title: '', year: '2023', media_type: 'tv', overview: '', rating: 0, total_episodes: 12, in_library_episodes: 12, watched_episodes: 12 },
            { tmdb_id: 3, title: '没看过的剧', original_title: '', year: '2022', media_type: 'tv', overview: '', rating: 0, total_episodes: 12, in_library_episodes: 12 },
          ],
          total_movies: 0, total_tv: 3, scanned_at: '2026-11-04T00:00:00Z', page: 1, page_size: 50,
        }),
      ),
    )
    render(
      <MemoryRouter>
        <LibraryPage filter="tv" />
      </MemoryRouter>,
    )
    expect(await screen.findByText('已看 3/12')).toBeInTheDocument()
    expect(await screen.findByText('已看完')).toBeInTheDocument()
  })

  it('电影卡片 plex_watched 显示「已看」徽标，未看不显示', async () => {
    const { server } = await import('@/test/mocks/server')
    server.use(
      http.get('/api/library', () =>
        HttpResponse.json({
          movies: [
            { tmdb_id: 10, title: '看过的电影', original_title: '', year: '2024', media_type: 'movie', overview: '', rating: 0, plex_watched: true },
            { tmdb_id: 11, title: '没看过的电影', original_title: '', year: '2023', media_type: 'movie', overview: '', rating: 0 },
          ],
          tv_shows: [],
          total_movies: 2, total_tv: 0, scanned_at: '2026-11-04T00:00:00Z', page: 1, page_size: 50,
        }),
      ),
    )
    render(
      <MemoryRouter>
        <LibraryPage filter="movies" />
      </MemoryRouter>,
    )
    expect(await screen.findByText('看过的电影')).toBeInTheDocument()
    expect(await screen.findByText('没看过的电影')).toBeInTheDocument()
    // 「已看」徽标只出现在看过的电影上，未看的不带徽标
    expect(screen.getAllByText('已看')).toHaveLength(1)
  })
})
