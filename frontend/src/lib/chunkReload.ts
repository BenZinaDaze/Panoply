import { lazy } from 'react'
import type { ComponentType } from 'react'

// 懒加载 chunk 失败自愈：前端重新构建后旧 hash chunk 已不存在，
// 长驻标签页切页时动态 import 会 404（报 Failed to fetch dynamically
// imported module / MIME text/html）。此时只能整页刷新拉取新 index.html。
// sessionStorage 时间戳保证 10 秒内最多刷新一次，避免服务端异常时陷入刷新死循环。
const CHUNK_RELOAD_KEY = 'chunk_reload_at'
const CHUNK_RELOAD_COOLDOWN_MS = 10_000

// loadable 带 chunk 失败自愈的 React.lazy 封装
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- 签名与 React.lazy 一致，保留原组件 props 类型
export function loadable<C extends ComponentType<any>>(
  load: () => Promise<{ default: C }>,
) {
  return lazy(() =>
    load().catch((err) => {
      const last = Number(sessionStorage.getItem(CHUNK_RELOAD_KEY) ?? 0)
      if (Date.now() - last > CHUNK_RELOAD_COOLDOWN_MS) {
        sessionStorage.setItem(CHUNK_RELOAD_KEY, String(Date.now()))
        window.location.reload()
      }
      throw err
    }),
  )
}
