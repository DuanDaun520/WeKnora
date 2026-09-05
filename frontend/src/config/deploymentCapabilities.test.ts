import assert from 'node:assert/strict'
import test from 'node:test'

import {
  CONSOLE_SECTION_CAPABILITY,
  SETTINGS_SECTION_CAPABILITY,
  isDeploymentCapabilitySupported,
  type DeploymentCapabilityMap,
} from './deploymentCapabilities'

test('capability filtering is fail-open unless backend explicitly disables a feature', () => {
  assert.equal(isDeploymentCapabilitySupported({}, 'organizations'), true)

  const capabilities: DeploymentCapabilityMap = {
    organizations: { supported: false, reason: 'not_supported_in_lite' },
    agents: { supported: true },
  }
  assert.equal(isDeploymentCapabilitySupported(capabilities, 'organizations'), false)
  assert.equal(isDeploymentCapabilitySupported(capabilities, 'agents'), true)
})

test('organizations stay hidden in lite even when capabilities fail open', () => {
  assert.equal(
    isDeploymentCapabilitySupported({}, 'organizations', { liteMode: true }),
    false,
  )
  assert.equal(
    isDeploymentCapabilitySupported({}, 'organizations', { edition: 'lite' }),
    false,
  )
  assert.equal(
    isDeploymentCapabilitySupported({}, 'agents', { liteMode: true }),
    true,
  )
})

test('only route-backed settings sections require deployment capabilities', () => {
  // 六个基础设施 section 已随 000095 迁入控制台，工作空间 Settings 只剩
  // envvars 与只读技能目录（000099 回迁）需要能力裁剪。
  assert.equal(SETTINGS_SECTION_CAPABILITY.envvars, 'settings.sandbox')
  assert.equal(SETTINGS_SECTION_CAPABILITY['skill-catalog'], 'settings.sandbox')
  for (const key of ['websearch', 'vectorstore', 'parser', 'storage', 'sandbox', 'mcp']) {
    assert.equal(
      Object.prototype.hasOwnProperty.call(SETTINGS_SECTION_CAPABILITY, key),
      false,
      `section ${key} should be governed by CONSOLE_SECTION_CAPABILITY`,
    )
  }
  assert.equal(SETTINGS_SECTION_CAPABILITY['runtime-queues'], undefined)
})

test('console-managed sections keep their capability keys after the 000095 move', () => {
  assert.equal(CONSOLE_SECTION_CAPABILITY.mcp, 'settings.mcp')
  assert.equal(CONSOLE_SECTION_CAPABILITY.storage, 'settings.storage')
  assert.equal(CONSOLE_SECTION_CAPABILITY.websearch, 'settings.websearch')
  assert.equal(CONSOLE_SECTION_CAPABILITY.vectorstore, 'settings.vectorstore')
  // 解析引擎没有独立能力键（后端未上报裁剪信息时保持可见）。
  assert.equal(CONSOLE_SECTION_CAPABILITY.parser, undefined)
  assert.equal(CONSOLE_SECTION_CAPABILITY['users'], undefined)
})

test('skill credentials follow the sandbox capability rather than a key of their own', () => {
  // The values are injected into a skill script's process, so a deployment with
  // no sandbox support has nowhere to put them and the page could only ever show
  // its empty state.
  assert.equal(SETTINGS_SECTION_CAPABILITY.envvars, 'settings.sandbox')
})

test('the console skill catalog panel moved back to workspace Settings', () => {
  // 000099：控制台技能代管面板移除，技能面收敛为「平台技能库（供给）+
  // 空间技能目录（消费）」，console 侧不再持有 skills 能力键；旧深链由
  // SystemConsole 跨路由改道到 /platform/settings?section=skill-catalog。
  assert.equal(CONSOLE_SECTION_CAPABILITY.skills, undefined)
})

test('the console sandbox config panel is merged into sandbox-connections', () => {
  // 沙箱配置面板并入「沙箱连接」后不再有独立 console section / 能力键：
  // 连接分配即物化配置，旧深链由 SystemConsole 改道。
  assert.equal(CONSOLE_SECTION_CAPABILITY.sandbox, undefined)
  assert.equal(CONSOLE_SECTION_CAPABILITY['sandbox-connections'], 'settings.sandbox')
})

test('the console sandbox-connections panel follows the sandbox capability', () => {
  // 平台沙箱连接（000097）沿用 settings.sandbox 能力门控：没有沙箱支持的
  // 部署里，连接目录同样没有可配置的内容。沙箱配置面板并入这里后，这里
  // 是沙箱面唯一的控制台入口。
  assert.equal(CONSOLE_SECTION_CAPABILITY['sandbox-connections'], 'settings.sandbox')
})

test('the console skill-library panel follows the sandbox capability', () => {
  // 平台技能库（000098）与技能目录共用 settings.sandbox 能力门控：技能
  // 只有装进沙箱镜像才有意义，没有沙箱支持的部署里技能库同样没有内容。
  assert.equal(CONSOLE_SECTION_CAPABILITY['skill-library'], 'settings.sandbox')
})

test('docker sandbox stays hidden unless the deployment explicitly enables it', () => {
  assert.equal(isDeploymentCapabilitySupported({}, 'settings.sandbox.docker'), false)
  assert.equal(
    isDeploymentCapabilitySupported(
      { 'settings.sandbox.docker': { supported: false, reason: 'docker_backend_disabled' } },
      'settings.sandbox.docker',
    ),
    false,
  )
  assert.equal(
    isDeploymentCapabilitySupported(
      { 'settings.sandbox.docker': { supported: true } },
      'settings.sandbox.docker',
    ),
    true,
  )
})
