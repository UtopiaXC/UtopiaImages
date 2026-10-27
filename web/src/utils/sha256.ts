/**
 * High-performance, streaming SHA-256 calculation for large files (GB+)
 * Uses WebCrypto for files <= 32MB, and pure streaming SHA-256 for larger files.
 * Memory overhead is constant O(1) (< 4MB) regardless of file size.
 */

const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
])

export class StreamingSHA256 {
  private h0 = 0x6a09e667
  private h1 = 0xbb67ae85
  private h2 = 0x3c6ef372
  private h3 = 0xa54ff53a
  private h4 = 0x510e527f
  private h5 = 0x9b05688c
  private h6 = 0x1f83d9ab
  private h7 = 0x5be0cd19

  private block = new Uint8Array(64)
  private blockLen = 0
  private totalBytes = 0
  private w = new Uint32Array(64)

  public update(chunk: Uint8Array): void {
    let offset = 0
    let len = chunk.length
    this.totalBytes += len

    while (len > 0) {
      if (this.blockLen === 0 && len >= 64) {
        this.processBlock(chunk, offset)
        offset += 64
        len -= 64
      } else {
        const toCopy = Math.min(64 - this.blockLen, len)
        this.block.set(chunk.subarray(offset, offset + toCopy), this.blockLen)
        this.blockLen += toCopy
        offset += toCopy
        len -= toCopy

        if (this.blockLen === 64) {
          this.processBlock(this.block, 0)
          this.blockLen = 0
        }
      }
    }
  }

  private processBlock(buf: Uint8Array, offset: number): void {
    const w = this.w
    for (let i = 0; i < 16; i++) {
      const idx = offset + (i << 2)
      w[i] = ((buf[idx] << 24) | (buf[idx + 1] << 16) | (buf[idx + 2] << 8) | buf[idx + 3]) >>> 0
    }

    for (let i = 16; i < 64; i++) {
      const x = w[i - 2]
      const gamma1 =
        ((x >>> 17) | (x << 15)) ^
        ((x >>> 19) | (x << 13)) ^
        (x >>> 10)
      const y = w[i - 15]
      const gamma0 =
        ((y >>> 7) | (y << 25)) ^
        ((y >>> 18) | (y << 14)) ^
        (y >>> 3)
      w[i] = (gamma1 + w[i - 7] + gamma0 + w[i - 16]) >>> 0
    }

    let a = this.h0
    let b = this.h1
    let c = this.h2
    let d = this.h3
    let e = this.h4
    let f = this.h5
    let g = this.h6
    let h = this.h7

    for (let i = 0; i < 64; i++) {
      const s1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7))
      const ch = (e & f) ^ (~e & g)
      const temp1 = (h + s1 + ch + K[i] + w[i]) >>> 0
      const s0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10))
      const maj = (a & b) ^ (a & c) ^ (b & c)
      const temp2 = (s0 + maj) >>> 0

      h = g
      g = f
      f = e
      e = (d + temp1) >>> 0
      d = c
      c = b
      b = a
      a = (temp1 + temp2) >>> 0
    }

    this.h0 = (this.h0 + a) >>> 0
    this.h1 = (this.h1 + b) >>> 0
    this.h2 = (this.h2 + c) >>> 0
    this.h3 = (this.h3 + d) >>> 0
    this.h4 = (this.h4 + e) >>> 0
    this.h5 = (this.h5 + f) >>> 0
    this.h6 = (this.h6 + g) >>> 0
    this.h7 = (this.h7 + h) >>> 0
  }

  public digest(): string {
    const totalBits = this.totalBytes * 8
    // Append 0x80 byte
    this.block[this.blockLen++] = 0x80

    if (this.blockLen > 56) {
      for (let i = this.blockLen; i < 64; i++) {
        this.block[i] = 0
      }
      this.processBlock(this.block, 0)
      this.blockLen = 0
    }

    for (let i = this.blockLen; i < 56; i++) {
      this.block[i] = 0
    }

    // High 32 bits of bit length
    const hiBits = Math.floor(totalBits / 0x100000000)
    const loBits = totalBits >>> 0

    this.block[56] = (hiBits >>> 24) & 0xff
    this.block[57] = (hiBits >>> 16) & 0xff
    this.block[58] = (hiBits >>> 8) & 0xff
    this.block[59] = hiBits & 0xff
    this.block[60] = (loBits >>> 24) & 0xff
    this.block[61] = (loBits >>> 16) & 0xff
    this.block[62] = (loBits >>> 8) & 0xff
    this.block[63] = loBits & 0xff

    this.processBlock(this.block, 0)

    const hex = (n: number) => n.toString(16).padStart(8, '0')
    return (
      hex(this.h0) +
      hex(this.h1) +
      hex(this.h2) +
      hex(this.h3) +
      hex(this.h4) +
      hex(this.h5) +
      hex(this.h6) +
      hex(this.h7)
    )
  }
}

/**
 * Calculates SHA-256 for a File or Blob with streaming reader and progress callback
 */
export async function computeFileSHA256(
  file: File | Blob,
  onProgress?: (percent: number) => void
): Promise<string> {
  // If file <= 32MB and WebCrypto is available, use native WebCrypto for max speed
  if (file.size <= 32 * 1024 * 1024 && window.crypto?.subtle) {
    if (onProgress) onProgress(30)
    const buf = await file.arrayBuffer()
    if (onProgress) onProgress(70)
    const hashBuf = await window.crypto.subtle.digest('SHA-256', buf)
    const hashArr = Array.from(new Uint8Array(hashBuf))
    if (onProgress) onProgress(100)
    return hashArr.map(b => b.toString(16).padStart(2, '0')).join('')
  }

  // Streaming calculation for large files (O(1) memory)
  const hasher = new StreamingSHA256()
  const chunkSize = 2 * 1024 * 1024 // 2MB slices
  let offset = 0
  const total = file.size

  while (offset < total) {
    const end = Math.min(offset + chunkSize, total)
    const slice = file.slice(offset, end)
    const buf = await slice.arrayBuffer()
    hasher.update(new Uint8Array(buf))
    offset = end

    if (onProgress) {
      onProgress(Math.min(99, Math.round((offset / total) * 100)))
    }
  }

  if (onProgress) onProgress(100)
  return hasher.digest()
}
