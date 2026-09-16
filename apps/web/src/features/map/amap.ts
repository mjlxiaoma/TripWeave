import AMapLoader from '@amap/amap-jsapi-loader'

// 高德 JS API 没有官方 TypeScript 类型，运行时对象统一以 any 在本模块边界传递。
/* eslint-disable @typescript-eslint/no-explicit-any */
export type AMapNS = any
export type AMapMap = any
export type AMapOverlay = any

let loaderPromise: Promise<AMapNS> | null = null

// hasMapKey 报告前端是否配置了高德 JS API key；未配置时地图组件渲染占位。
export function hasMapKey(): boolean {
  return Boolean(import.meta.env.VITE_AMAP_KEY)
}

// loadAMap 单例加载 JS API（多组件共享一个 promise）。未配置 key 返回 null。
// 失败时清空缓存的 promise，允许下次渲染重试。
export function loadAMap(): Promise<AMapNS> | null {
  const key = import.meta.env.VITE_AMAP_KEY as string | undefined
  if (!key) return null
  // securityJsCode 必须在 loader 执行前挂到 window（高德 2021-12 后强制）
  const code = import.meta.env.VITE_AMAP_SECURITY_CODE as string | undefined
  if (code) {
    ;(window as any)._AMapSecurityConfig = { securityJsCode: code }
  }
  loaderPromise ??= AMapLoader.load({
    key,
    version: '2.0',
    plugins: ['AMap.Marker', 'AMap.Polyline', 'AMap.InfoWindow', 'AMap.Scale'],
  }).catch((err) => {
    loaderPromise = null
    throw err
  })
  return loaderPromise
}
