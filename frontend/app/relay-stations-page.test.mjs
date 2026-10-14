import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import vm from 'node:vm'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const { build } = createRequire(require.resolve('vite'))('esbuild')
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const station = { id: 1, type: 'newapi', name: 'NewAPI', base_url: 'https://relay.example',
  api_key_configured: true, gateway_key_configured: false, auth_mode: 'password', username: 'admin', admin_user_id: '1',
  auto_adjust_enabled: false, auto_adjust_no_profit_enabled: false, auto_priority_enabled: false,
  auto_priority_recall_enabled: false, auto_priority_recall_minutes: 180, last_synced_at: '2026-10-14T00:00:00Z' }
const summary = { account_count: 0, group_count: 0, channel_count: 0, risk_count: 0, adjustable_count: 0, no_profit_count: 0, unknown_cost_count: 0, healthy_count: 0, disabled_count: 0 }
const overview = { station, accounts: [], groups: [], summary, adjustments: [], monitor_channels: [], sync_available: true, range: 'today', usage_complete: true, usage_failed_accounts: 0 }
const context = vm.createContext({ console, URL, URLSearchParams, AbortController, setTimeout, clearTimeout,
  window: { localStorage: { getItem: () => null, setItem() {} } }, __relayData: { station, overview } })
const hookNames = [...readFileSync(path.join(root, 'lib/queries.ts'), 'utf8').matchAll(/export function (use\w+)\(/g)].map((match) => match[1])
const queryMock = hookNames.map((name) => `export function ${name}() {
  const fixture = globalThis.__relayData;
  const data = '${name}' === 'useRelayStations' ? [fixture.station]
    : '${name}' === 'useRelayOverview' ? fixture.overview
    : '${name}' === 'useRelayUserBalanceHistory' ? null
    : '${name}' === 'useChannelRelayAssociations' ? { stations: [], links: [] }
    : '${name}' === 'useRelayUsage' ? { accounts: [], range: 'today', complete: true, failed_accounts: 0 }
    : '${name}' === 'useRelayUsers' ? { items: [], total: 0, total_balance: 0, page: 1, page_size: 20, pages: 1, complete: true }
    : [];
  return { data, loading: false, refreshing: false, error: null, refetch: async () => {} };
}`).join('\n')
const bundle = await build({ entryPoints: [path.join(root, 'app/relay-stations-page.tsx')],
  bundle: true, write: false, format: 'cjs', platform: 'node', packages: 'external', alias: { '@': root }, jsx: 'automatic',
  plugins: [{ name: 'relay-query-fixtures', setup(builder) {
    builder.onResolve({ filter: /^@\/lib\/queries$/ }, () => ({ path: 'queries', namespace: 'fixture' }))
    builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({ contents: queryMock, loader: 'js' }))
  } }], })
context.module = { exports: {} }
context.exports = context.module.exports
context.require = require
vm.runInContext(bundle.outputFiles[0].text, context)
const page = context.module.exports
const json = (value) => JSON.parse(JSON.stringify(value))
const form = (patch = {}) => ({ type: 'newapi', auth_mode: 'password', name: 'NewAPI', base_url: 'https://relay.example', api_key: '', username: 'admin', password: 'secret', admin_token: '', admin_cookie: '', admin_user_id: '1', gateway_key: '', ...patch })

test('\u521b\u5efa\u4ec5\u53d1\u9001\u6240\u9009\u9274\u6743\u5b57\u6bb5', () => {
  assert.equal(typeof page.buildRelayStationPayload, 'function')
  assert.deepEqual(json(page.buildRelayStationPayload(form())), { type: 'newapi', auth_mode: 'password', name: 'NewAPI', base_url: 'https://relay.example', username: 'admin', password: 'secret' })
  assert.deepEqual(json(page.buildRelayStationPayload(form({ auth_mode: 'access_token', admin_token: 'token', gateway_key: 'sk-key' }))), { type: 'newapi', auth_mode: 'access_token', name: 'NewAPI', base_url: 'https://relay.example', admin_token: 'token', admin_user_id: '1', gateway_key: 'sk-key' })
  assert.deepEqual(json(page.buildRelayStationPayload(form({ auth_mode: 'cookie', admin_cookie: 'session=secret' }))), { type: 'newapi', auth_mode: 'cookie', name: 'NewAPI', base_url: 'https://relay.example', admin_cookie: 'session=secret', admin_user_id: '1' })
  assert.deepEqual(json(page.buildRelayStationPayload(form({ type: 'sub2api', api_key: 'x-api-key' }))), { type: 'sub2api', name: 'NewAPI', base_url: 'https://relay.example', api_key: 'x-api-key' })
})

test('\u6458\u8981\u548c\u8bf7\u6c42\u7c7b\u578b\u5339\u914d NewAPI \u5951\u7ea6', () => {
  const contract = `import type { RelayGroupView, RelayStation, RelayStationPayload } from './lib/api-types';
    type Assert<T extends true> = T;
    type HasType = Assert<RelayStation['type'] extends 'newapi' | 'sub2api' ? true : false>;
    type HasAuth = Assert<NonNullable<RelayStation['auth_mode']> extends 'password' | 'access_token' | 'cookie' ? true : false>;
    type HasLocalSort = Assert<RelayGroupView['sort_order'] extends number | undefined ? true : false>;
    type NoSecrets = Assert<Extract<keyof RelayStation, 'password' | 'admin_token' | 'admin_cookie' | 'gateway_key'> extends never ? true : false>;
    const summary: Pick<RelayStation, 'type' | 'auth_mode' | 'username' | 'admin_user_id' | 'gateway_key_configured'> = {
      type: 'newapi', auth_mode: 'cookie', username: 'admin', admin_user_id: '1', gateway_key_configured: true };
    const payload: RelayStationPayload = { type: 'newapi', name: 'relay', base_url: 'https://relay.example',
      auth_mode: 'cookie', admin_cookie: 'session=secret', admin_user_id: '1', gateway_key: 'sk-key' };`
  const virtualPath = path.join(root, 'relay-contract.type-test.ts')
  const host = ts.createCompilerHost({})
  const originalGetSource = host.getSourceFile.bind(host)
  host.getSourceFile = (file, language, ...rest) => path.resolve(file) === virtualPath ? ts.createSourceFile(file, contract, language) : originalGetSource(file, language, ...rest)
  const program = ts.createProgram([virtualPath], { strict: true, noEmit: true, skipLibCheck: true, moduleResolution: ts.ModuleResolutionKind.Bundler, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 }, host)
  const diagnostics = ts.getPreEmitDiagnostics(program)
  assert.equal(diagnostics.length, 0, ts.formatDiagnostics(diagnostics, { getCurrentDirectory: () => root, getCanonicalFileName: (file) => file, getNewLine: () => '\n' }))
})

test('\u7f16\u8f91\u56de\u586b\u516c\u5f00\u5b57\u6bb5\u4e14\u7559\u7a7a\u4fdd\u7559\u79d8\u5bc6', () => {
  assert.equal(typeof page.createRelayStationForm, 'function')
  const editing = page.createRelayStationForm(station)
  assert.equal(editing.username, 'admin')
  assert.equal(editing.admin_user_id, '1')
  assert.equal(editing.password, '')
  assert.deepEqual(json(page.buildRelayStationPayload(editing, station)), { type: 'newapi', auth_mode: 'password', name: 'NewAPI', base_url: 'https://relay.example', username: 'admin' })
  for (const [mode, secret] of [['access_token', 'admin_token'], ['cookie', 'admin_cookie']]) {
    const existing = { ...station, auth_mode: mode }
    const payload = page.buildRelayStationPayload(page.createRelayStationForm(existing), existing)
    assert.equal(payload.auth_mode, mode)
    assert.equal(secret in payload, false)
    assert.equal('gateway_key' in payload, false)
  }
  assert.equal('api_key' in page.buildRelayStationPayload(form({ type: 'sub2api', api_key: '   ' }), { ...station, type: 'sub2api' }), false)
})

test('\u5207\u6362\u6a21\u5f0f\u9700\u8981\u65b0\u79d8\u5bc6\uff0c\u7f51\u5173 Key \u4e0d\u66ff\u4ee3\u7ba1\u7406\u5458\u51ed\u636e', () => {
  assert.equal(typeof page.buildRelayStationPayload, 'function')
  assert.throws(() => page.buildRelayStationPayload(form({ password: '', gateway_key: 'sk-key' })), /\u7ba1\u7406\u5458/)
  assert.throws(() => page.buildRelayStationPayload(form({ auth_mode: 'cookie', admin_cookie: '', gateway_key: 'sk-key' }), station), /Cookie/)
  assert.throws(() => page.buildRelayStationPayload(form({ auth_mode: 'access_token', admin_token: 'token', admin_user_id: '0' })), /\u7528\u6237 ID/)
  assert.equal(page.buildRelayStationPayload(form({ username: '', password: '' }), station).username, 'admin')
})

test('\u9690\u85cf\u4e13\u5c5e\u80fd\u529b\u4f46\u4fdd\u7559\u5206\u7ec4\u6d4b\u8bd5\u548c\u4f18\u5148\u7ea7', () => {
  assert.equal(typeof page.relayStationCapabilities, 'function')
  const capabilities = page.relayStationCapabilities('newapi')
  for (const key of ['accountConcurrency', 'poolRetry', 'userLimits']) assert.equal(capabilities[key], false, key)
  for (const key of ['groupTest', 'priority', 'scheduling', 'groupRate', 'userStatus', 'costBinding', 'monitoring']) assert.equal(capabilities[key], true, key)
  assert.equal(page.relayStationCapabilities('sub2api').accountConcurrency, true)
  assert.equal(page.relayPriorityRequiresOAuth('sub2api', 1, 'apikey'), true)
  assert.equal(page.relayPriorityRequiresOAuth('newapi', 1, 'apikey'), false)
})

test('\u8868\u5355\u6e32\u67d3\u7c7b\u578b\u548c\u4e09\u79cd\u9274\u6743\u5b57\u6bb5', () => {
  assert.equal(typeof page.RelayStationFields, 'function')
  const render = (value, existing) => renderToStaticMarkup(React.createElement(page.RelayStationFields, { form: value, station: existing, onChange() {} }))
  const password = render(form())
  for (const id of ['relay-type', 'relay-username', 'relay-password', 'relay-gateway-key']) assert.match(password, new RegExp(id))
  assert.doesNotMatch(password, /id="relay-admin-token"/)
  const token = render(form({ auth_mode: 'access_token' }))
  assert.match(token, /relay-admin-token/)
  assert.match(token, /relay-admin-user-id/)
  assert.doesNotMatch(token, /id="relay-password"/)
  assert.match(render(form({ auth_mode: 'cookie' })), /relay-admin-cookie/)
  assert.match(render(form({ type: 'sub2api' })), /relay-key/)
  assert.doesNotMatch(render(form({ type: 'sub2api' })), /relay-gateway-key/)
})

test('\u9519\u8bef\u533a\u5206\u7ba1\u7406\u5458\u9274\u6743\u4e0e\u7f51\u5173\u8c03\u7528', () => {
  assert.equal(typeof page.relayStationErrorMessage, 'function')
  assert.equal(typeof page.buildRelayStationErrorMessage, 'function')
  const loginError = page.buildRelayStationErrorMessage('newapi', 'sync', 'HTTP 401 Unauthorized')
  assert.match(loginError, /\u7ba1\u7406\u5458\u9274\u6743\u5931\u8d25/)
  assert.doesNotMatch(loginError, /\u771f\u5b9e\u5206\u7ec4\u8c03\u7528\u5931\u8d25/)
  const adminError = page.relayStationErrorMessage('newapi', 'group-models', 'HTTP 401')
  assert.match(adminError, /\u7ba1\u7406\u5458\u51ed\u636e/)
  assert.match(adminError, /\u4e0d\u9700\u8981 Gateway API Key/)
  const message = page.relayStationErrorMessage('newapi', 'group-test', 'gateway key missing')
  assert.match(message, /\u7f51\u5173 API Key/)
  assert.match(message, /\u5206\u7ec4/)
  assert.match(message, /\u5f53\u524d\u7ba1\u7406\u5458\u540d\u4e0b/)
  assert.match(message, /\u7559\u7a7a.*\u81ea\u52a8\u9009\u62e9/)
  assert.match(message, /\u7ba1\u7406\u5458\u51ed\u636e/)
  const groupCallError = page.buildRelayStationErrorMessage('newapi', 'group-test', 'HTTP 401 Unauthorized')
  assert.match(groupCallError, /\u771f\u5b9e\u5206\u7ec4\u8c03\u7528\u5931\u8d25/)
  assert.doesNotMatch(groupCallError, /\u7ba1\u7406\u5458\u9274\u6743\u5931\u8d25/)
})

test('\u5217\u8868\u548c\u56fe\u518c\u4ec5\u5c55\u793a\u5f53\u524d\u7ad9\u70b9\u652f\u6301\u7684\u80fd\u529b', () => {
  assert.equal(typeof page.stationAccountColumns, 'function')
  const columns = page.stationAccountColumns('newapi').map((column) => column.key)
  assert.equal(page.relayStationCapabilities('newapi').accountCostProbe, false)
  for (const key of ['concurrency', 'capacity', 'retry']) assert.equal(columns.includes(key), false)
  for (const key of ['priority', 'schedulable', 'cost']) assert.equal(columns.includes(key), true)
  assert.equal(typeof page.RelayAccountGalleryCard, 'function')
  const account = { external_id: 1, name: 'channel', platform: 'newapi', type: 'apikey', risk_state: 'cost_unknown', schedulable: true, priority: 5, pool_mode_retry_count: 3, concurrency: 9, current_concurrency: 1, current_groups: [], suggestions: [], downgrade_groups: [], latency_samples: [], candidate_groups: [] }
  const html = renderToStaticMarkup(React.createElement(page.RelayAccountGalleryCard, { account, overview, allowProbe: false, healthScore: null, stabilityBuckets: [], consumptionBuckets: [], channelLinks: [], channelLatencyBucketsByChannel: new Map(), channelRecentSamplesByChannel: new Map(), channelConsumptionBucketsByChannel: new Map(), onToggle() {}, onSchedulableChange() {}, onApply() {}, onAddDowngrade() {}, onEditGroups() {}, onProbe() {}, onTest() {}, onDelete() {} }))
  assert.doesNotMatch(html, /\u91cd\u8bd5|\u5e76\u53d1|\u5bb9\u91cf/)
  assert.doesNotMatch(html, /aria-label="\u63a2\u6d4b\u500d\u7387"/)
  assert.match(html, /\u4f18\u5148\u7ea7/)
  assert.match(html, /\u5f00\u542f.*\u8c03\u5ea6|\u5173\u95ed.*\u8c03\u5ea6/)
})

test('\u5207\u6362\u7ad9\u70b9\u65f6\u4e0d\u4fdd\u7559\u4e0d\u652f\u6301\u7684\u6279\u91cf\u6a21\u5f0f', () => {
  assert.equal(typeof page.relayBatchModeForStation, 'function')
  assert.equal(page.relayBatchModeForStation('newapi', 'concurrency'), 'channel_group')
  assert.equal(page.relayBatchModeForStation('newapi', 'retry_count'), 'channel_group')
  assert.equal(page.relayBatchModeForStation('newapi', 'priority'), 'priority')
  assert.equal(page.relayBatchModeForStation('sub2api', 'retry_count'), 'retry_count')
})

test('\u9875\u9762\u5c55\u793a type \u4e14\u9690\u85cf\u5e76\u53d1\u6c60\u91cd\u8bd5', () => {
  context.window.localStorage.getItem = (key) => key === 'uh_relay_detail_module' ? 'accounts' : null
  const html = renderToStaticMarkup(React.createElement(page.default))
  assert.match(html, /data-station-type="newapi"/)
  assert.doesNotMatch(html, /\u6279\u91cf\u8bbe\u7f6e\u5e76\u53d1|\u6c60\u6a21\u5f0f|\u540c\u8d26\u53f7\u91cd\u8bd5/)
  assert.equal(typeof page.NewAPIUserManagement, 'function')
  const users = renderToStaticMarkup(React.createElement(page.NewAPIUserManagement, { stationID: 1, range: 'today' }))
  assert.doesNotMatch(users, /RPM|\u5e76\u53d1\u4e0a\u9650|\u5f53\u524d\u5e76\u53d1/)
  assert.match(users, /\u9009\u62e9\u5f53\u524d\u9875\u666e\u901a\u7528\u6237/)
})

test('NewAPI \u5206\u7ec4\u4fdd\u7559\u7f16\u8f91\u4e0e\u5feb\u901f\u6d4b\u8bd5\uff0c\u540d\u79f0\u53ea\u8bfb\u4e0e\u72b6\u6001\u5c55\u793a', () => {
  assert.equal(typeof page.GroupManagementRow, 'function')
  const group = { external_id: 1, name: 'default', description: '', rate_multiplier: 1, model_types: [], account_types: ['apikey'], status: 'active', monitor_enabled: true, is_exclusive: false, channels: [] }
  const html = renderToStaticMarkup(React.createElement(page.GroupManagementRow, { group, accounts: [], stationType: 'newapi', onUpdate() {}, onTest() {}, onDelete() {}, onDragStart() {}, onDragOver() {}, onDrop() {}, busy: false }))
  const nameInput = html.match(/<input(?=[^>]*readonly="")(?=[^>]*value="default")[^>]*>/i)?.[0]
  assert.ok(nameInput)
  assert.doesNotMatch(nameInput, /disabled=/)
  assert.doesNotMatch(html, /aria-label="default\u505c\u7528"/)
  assert.match(html, /\u5feb\u901f\u6d4b\u8bd5/)
  assert.match(html, /root/)
  assert.doesNotMatch(html, /aria-label="default\u5206\u7ec4\u63cf\u8ff0"[^>]*disabled=""/)
  const exclusiveSwitch = html.match(/<button(?=[^>]*aria-label="default\u5207\u6362\u4e3a\u4e13\u5c5e\u5206\u7ec4")[^>]*>/)?.[0]
  const monitorSwitch = html.match(/<button(?=[^>]*aria-label="default\u5173\u95ed\u516c\u5f00\u76d1\u63a7")[^>]*>/)?.[0]
  assert.ok(exclusiveSwitch)
  assert.ok(monitorSwitch)
  assert.doesNotMatch(exclusiveSwitch, /disabled=/)
  assert.doesNotMatch(monitorSwitch, /disabled=/)
  assert.deepEqual(json(page.buildRelayGroupPatch('newapi', group, { name: 'renamed', description: 'local', rate_multiplier: 1, model_types: ['gpt'] })), { description: 'local', model_types: ['gpt'] })
  assert.deepEqual(json(page.buildRelayGroupPatch('newapi', group, { name: 'default', description: '', rate_multiplier: 2, model_types: [] })), { description: '', model_types: [], rate_multiplier: 2 })
  assert.deepEqual(json(page.buildRelayGroupUpdates('newapi', group, { name: 'default', description: 'local', rate_multiplier: 2, model_types: ['gpt'] })), [{ description: 'local', model_types: ['gpt'] }, { rate_multiplier: 2 }])
})
