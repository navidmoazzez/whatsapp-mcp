#!/usr/bin/env node
/**
 * Downloads the prebuilt binary for this platform from GitHub Releases.
 *
 * The server is Go, not Node. This package exists so people can run it with
 * npx without installing a Go toolchain, which is otherwise the single biggest
 * barrier to trying it.
 *
 * Nothing is compiled here. The binary is pure Go with no cgo, so one build
 * per platform covers everyone.
 */

import { createWriteStream, existsSync, mkdirSync, chmodSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { pipeline } from 'node:stream/promises'
import { createGunzip } from 'node:zlib'

const here = dirname(fileURLToPath(import.meta.url))
const version = JSON.parse(
  await import('node:fs').then((fs) =>
    fs.promises.readFile(join(here, '..', 'package.json'), 'utf8'),
  ),
).version

const TARGETS = {
  'darwin-arm64': 'darwin_arm64',
  'darwin-x64': 'darwin_amd64',
  'linux-arm64': 'linux_arm64',
  'linux-x64': 'linux_amd64',
  'win32-x64': 'windows_amd64',
}

const key = `${process.platform}-${process.arch}`
const target = TARGETS[key]

if (!target) {
  console.error(
    `whatsapp-mcp: no prebuilt binary for ${key}.\n` +
      `Build from source instead: go install github.com/navidmoazzez/whatsapp-mcp/cmd/whatsapp-mcp@latest`,
  )
  process.exit(0) // Do not fail the install; the error message is the useful part.
}

const exe = process.platform === 'win32' ? 'whatsapp-mcp.exe' : 'whatsapp-mcp'
const dest = join(here, exe)

if (existsSync(dest)) process.exit(0)

const url =
  `https://github.com/navidmoazzez/whatsapp-mcp/releases/download/` +
  `v${version}/whatsapp-mcp_${target}.gz`

try {
  const res = await fetch(url, { redirect: 'follow' })
  if (!res.ok) throw new Error(`${res.status} for ${url}`)

  mkdirSync(here, { recursive: true })
  await pipeline(res.body, createGunzip(), createWriteStream(dest))
  if (process.platform !== 'win32') chmodSync(dest, 0o755)
} catch (err) {
  console.error(
    `whatsapp-mcp: could not download the binary (${err.message}).\n` +
      `Build from source instead: go install github.com/navidmoazzez/whatsapp-mcp/cmd/whatsapp-mcp@latest`,
  )
  process.exit(0)
}
