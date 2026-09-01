#!/usr/bin/env node
/** Runs the platform binary, passing every argument and signal straight through. */

import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const exe = join(here, process.platform === 'win32' ? 'whatsapp-mcp.exe' : 'whatsapp-mcp')

if (!existsSync(exe)) {
  console.error(
    'whatsapp-mcp: the binary is missing. Reinstall, or build from source:\n' +
      '  go install github.com/navidmoazzez/whatsapp-mcp/cmd/whatsapp-mcp@latest',
  )
  process.exit(1)
}

// stdio inherit matters: this server speaks MCP over stdin and stdout, so the
// streams have to be the real ones rather than pipes.
const child = spawn(exe, process.argv.slice(2), { stdio: 'inherit' })
for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => child.kill(sig))
}
child.on('exit', (code, signal) => process.exit(signal ? 1 : (code ?? 0)))
