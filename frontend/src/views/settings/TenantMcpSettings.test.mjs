import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./TenantMcpSettings.vue', import.meta.url), 'utf8')

test('tenant mcp settings is a read-only view of the effective workspace list', () => {
  assert.match(source, /listMCPServices/)
  assert.match(source, /tenantMcpSettings\.title/)
  assert.match(source, /tenantMcpSettings\.empty/)
  assert.match(source, /tenantMcpSettings\.loadFailed/)
  assert.match(source, /tenantMcpSettings\.readonlyHint/)
  // 写动作留在系统管理控制台，空间侧不得出现任何 MCP 写接口调用。
  assert.doesNotMatch(source, /createMCPService|updateMCPService|deleteMCPService|testMCPService/)
})

test('transport types render as human labels with status and builtin badges', () => {
  assert.match(source, /http-streamable/)
  assert.match(source, /mcp-status-dot/)
  assert.match(source, /tenantMcpSettings\.builtin/)
  assert.match(source, /svc\.enabled/)
})
