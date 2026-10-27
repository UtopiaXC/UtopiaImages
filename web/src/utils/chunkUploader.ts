import { api } from '../api/client'
import { computeFileSHA256 } from './sha256'

export interface ChunkUploadProgress {
  phase: 'hashing' | 'uploading' | 'assembling'
  percent: number
  currentChunk?: number
  totalChunks?: number
  text: string
}

export interface ChunkUploadOptions {
  file: File
  uploadType: 'file' | 'archive'
  chunkSize?: number // default 20MB
  params?: {
    policy_id?: number | null
    watermark_mode?: string
    watermark_template_id?: string | null
    hotlink_mode?: string
    preserve_path?: boolean
    strip_top_dir?: boolean
    conflict_action?: string
  }
  onProgress?: (progress: ChunkUploadProgress) => void
  abortController?: AbortController
}

export const DEFAULT_CHUNK_SIZE = 20 * 1024 * 1024 // 20 MB

/**
 * Uploads a file or archive in 20MB chunks, verifies SHA-256 on backend,
 * and cleans up cache on abort/failure.
 */
export async function uploadInChunks(options: ChunkUploadOptions): Promise<any> {
  const { file, uploadType, params = {}, onProgress, abortController } = options
  const chunkSize = options.chunkSize || DEFAULT_CHUNK_SIZE
  const totalChunks = Math.max(1, Math.ceil(file.size / chunkSize))
  let uploadId: string | null = null

  const checkAborted = () => {
    if (abortController?.signal?.aborted) {
      throw new Error('Upload cancelled by user')
    }
  }

  try {
    // 1. Calculate SHA-256 with streaming
    checkAborted()
    if (onProgress) {
      onProgress({
        phase: 'hashing',
        percent: 0,
        text: '正在计算文件 SHA-256 校验码...'
      })
    }

    const fileSHA256 = await computeFileSHA256(file, (percent) => {
      checkAborted()
      if (onProgress) {
        onProgress({
          phase: 'hashing',
          percent,
          text: `正在计算 SHA-256 校验码 (${percent}%)...`
        })
      }
    })

    checkAborted()

    // 2. Initialize chunk upload
    if (onProgress) {
      onProgress({
        phase: 'uploading',
        percent: 0,
        currentChunk: 0,
        totalChunks,
        text: '正在初始化分块上传...'
      })
    }

    const initPayload = {
      filename: file.name,
      total_size: file.size,
      chunk_size: chunkSize,
      total_chunks: totalChunks,
      file_sha256: fileSHA256,
      upload_type: uploadType,
      policy_id: params.policy_id || undefined,
      watermark_mode: params.watermark_mode || 'inherit',
      watermark_template_id: params.watermark_template_id ? Number(params.watermark_template_id) : undefined,
      hotlink_mode: params.hotlink_mode || 'inherit',
      preserve_path: params.preserve_path ?? true,
      strip_top_dir: params.strip_top_dir ?? false,
      conflict_action: params.conflict_action || 'skip'
    }

    const initRes = await api.post('/upload/chunk/init', initPayload)
    uploadId = initRes.data.upload_id
    const uploadedSet = new Set<number>(initRes.data.uploaded_chunks || [])

    // 3. Upload chunks sequentially or in controlled concurrency
    for (let i = 0; i < totalChunks; i++) {
      checkAborted()

      if (uploadedSet.has(i)) {
        if (onProgress) {
          const pct = Math.round(((i + 1) / totalChunks) * 100)
          onProgress({
            phase: 'uploading',
            percent: pct,
            currentChunk: i + 1,
            totalChunks,
            text: `分块 ${i + 1}/${totalChunks} 已存在，跳过...`
          })
        }
        continue
      }

      const start = i * chunkSize
      const end = Math.min(start + chunkSize, file.size)
      const chunkBlob = file.slice(start, end)

      const formData = new FormData()
      formData.append('upload_id', uploadId!)
      formData.append('chunk_index', String(i))
      formData.append('file', chunkBlob, `${file.name}.part${i}`)

      let retries = 3
      while (retries > 0) {
        checkAborted()
        try {
          if (onProgress) {
            const pct = Math.round((i / totalChunks) * 100)
            onProgress({
              phase: 'uploading',
              percent: pct,
              currentChunk: i + 1,
              totalChunks,
              text: `正在上传分块 ${i + 1}/${totalChunks} (${pct}%)...`
            })
          }

          await api.post('/upload/chunk/part', formData, {
            timeout: 0,
            signal: abortController?.signal
          })
          break
        } catch (err: any) {
          checkAborted()
          retries--
          if (retries === 0) {
            throw new Error(`分块 ${i + 1}/${totalChunks} 上传失败: ${err.message || err}`)
          }
          await new Promise((r) => setTimeout(r, 1000))
        }
      }

      if (onProgress) {
        const pct = Math.round(((i + 1) / totalChunks) * 100)
        onProgress({
          phase: 'uploading',
          percent: pct,
          currentChunk: i + 1,
          totalChunks,
          text: `分块 ${i + 1}/${totalChunks} 上传完成 (${pct}%)`
        })
      }
    }

    // 4. Complete / Merge & SHA256 verify
    checkAborted()
    if (onProgress) {
      onProgress({
        phase: 'assembling',
        percent: 100,
        text: '所有分块上传完成，服务端正在流式组装并校验 SHA-256...'
      })
    }

    const completeRes = await api.post('/upload/chunk/complete', {
      upload_id: uploadId
    }, {
      timeout: 0,
      signal: abortController?.signal
    })

    return completeRes.data
  } catch (err: any) {
    // If upload was initialized, abort task and clean up cache on backend
    if (uploadId) {
      try {
        await api.post('/upload/chunk/abort', { upload_id: uploadId })
      } catch (cleanErr) {
        console.warn('Failed to abort chunk upload on error', cleanErr)
      }
    }
    throw err
  }
}
