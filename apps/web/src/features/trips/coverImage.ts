// 封面图片前端压缩:最长边 ≤1600px、JPEG quality 0.82,产物通常 ≤300KB。
// 压缩在上传前做,弱网友好;服务器只收单一规格,无需多档裁剪。
const MAX_EDGE = 1600
const QUALITY = 0.82
const OUTPUT_TYPE = 'image/jpeg'

export interface CompressedCover {
  blob: Blob
  /** dataURL 预览(避免再挂 blob URL) */
  previewUrl: string
}

export async function compressCover(file: File): Promise<CompressedCover> {
  if (!file.type.startsWith('image/')) {
    throw new Error('not an image')
  }
  const src = await fileToDataUrl(file)
  const img = await loadImage(src)
  const scale = Math.min(1, MAX_EDGE / Math.max(img.naturalWidth, img.naturalHeight))
  const w = Math.max(1, Math.round(img.naturalWidth * scale))
  const h = Math.max(1, Math.round(img.naturalHeight * scale))
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas unsupported')
  ctx.drawImage(img, 0, 0, w, h)
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('encode failed'))), OUTPUT_TYPE, QUALITY)
  })
  return { blob, previewUrl: src }
}

function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(new Error('read failed'))
    reader.readAsDataURL(file)
  })
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('decode failed'))
    img.src = src
  })
}
