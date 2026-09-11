<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card p-4 sm:p-6">
        <div class="flex flex-wrap items-end gap-3">
          <div ref="userSearchRef" class="relative min-w-[220px] flex-1"><label class="input-label">{{ t('admin.requestRecords.user') }}</label><input v-model="userKeyword" class="input pr-8" :placeholder="t('admin.requestRecords.searchUserPlaceholder')" @input="debounceUserSearch" @focus="showUserDropdown = true" /><button v-if="filters.user_id" type="button" class="absolute right-2 top-9 text-gray-400" @click="clearUser" aria-label="Clear user filter">✕</button><div v-if="showUserDropdown && (userResults.length > 0 || userKeyword)" class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-dark-800"><button v-for="user in userResults" :key="user.id" type="button" class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-dark-700" @click="selectUser(user)"><span>{{ user.email }}<span v-if="user.deleted" class="ml-1 text-xs text-gray-400">（{{ t('admin.requestRecords.deleted') }}）</span></span><span class="ml-2 text-xs text-gray-400">#{{ user.id }}</span></button></div></div>
          <div ref="apiKeySearchRef" class="relative min-w-[220px] flex-1"><label class="input-label">{{ t('admin.requestRecords.apiKey') }}</label><input v-model="apiKeyKeyword" class="input pr-8" :placeholder="t('admin.requestRecords.searchApiKeyPlaceholder')" @input="debounceApiKeySearch" @focus="onApiKeyFocus" /><button v-if="filters.api_key_id" type="button" class="absolute right-2 top-9 text-gray-400" @click="clearApiKey" aria-label="Clear API key filter">✕</button><div v-if="showApiKeyDropdown && apiKeyResults.length > 0" class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-dark-800"><button v-for="key in apiKeyResults" :key="key.id" type="button" class="w-full px-4 py-2 text-left hover:bg-gray-100 dark:hover:bg-dark-700" @click="selectApiKey(key)"><span>{{ key.name || `#${key.id}` }}</span><span class="ml-2 text-xs text-gray-400">#{{ key.id }}</span></button></div></div>
          <div class="min-w-[220px] flex-1"><label class="input-label">{{ t('admin.requestRecords.model') }}</label><Select v-model="filters.model" :options="modelOptions" searchable clearable creatable :creatable-prefix="t('common.search')" @change="search" /></div>
          <div class="min-w-[220px] flex-1"><label class="input-label">{{ t('admin.requestRecords.path') }}</label><input v-model.trim="filters.path" class="input" @keyup.enter="search" /></div>
          <div class="w-36"><label class="input-label">{{ t('admin.requestRecords.method') }}</label><Select v-model="filters.method" :options="methodOptions" @change="search" /></div>
          <div class="w-40"><label class="input-label">{{ t('admin.requestRecords.status') }}</label><Select v-model="filters.status_code" :options="statusOptions" searchable @change="search" /></div>
          <div class="w-44"><label class="input-label">{{ t('admin.requestRecords.startTime') }}</label><input v-model="filters.start_date" type="date" class="input" @change="search" /></div>
          <div class="w-44"><label class="input-label">{{ t('admin.requestRecords.endTime') }}</label><input v-model="filters.end_date" type="date" class="input" @change="search" /></div>
          <div class="flex gap-2"><button class="btn btn-primary" :disabled="loading" @click="search">{{ t('common.search') }}</button><button class="btn btn-secondary" :disabled="loading" @click="reset">{{ t('common.reset') }}</button><button class="btn btn-secondary" :disabled="exporting" @click="download">{{ exporting ? t('admin.requestRecords.exporting') : t('admin.requestRecords.export') }}</button></div>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-6">
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ total }} {{ t('admin.requestRecords.records') }}</span>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary px-3" type="button" :disabled="!filters.api_key_id || conversationLoading" @click="conversationMode ? closeConversation() : openConversation()">{{ conversationMode ? t('admin.requestRecords.listView') : t('admin.requestRecords.conversationView') }}</button>
            <div v-if="!conversationMode" ref="columnDropdownRef" class="relative">
              <button class="btn btn-secondary px-3" type="button" @click="showColumnDropdown = !showColumnDropdown">{{ t('admin.requestRecords.columns') }}</button>
              <div v-if="showColumnDropdown" class="absolute right-0 z-50 mt-1 max-h-80 w-56 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800">
                <button v-for="column in toggleableColumns" :key="column.key" type="button" class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="toggleColumn(column.key)"><span>{{ column.label }}</span><span v-if="isColumnVisible(column.key)" class="text-primary-500">✓</span></button>
              </div>
            </div>
          </div>
        </div>

        <DataTable v-if="!conversationMode" :columns="visibleColumns" :data="records" :loading="loading" row-key="id" clickable-rows @row-click="openDetail">
          <template #cell-user="{ row }"><div class="min-w-[180px] max-w-[240px]"><div class="truncate font-medium text-gray-900 dark:text-white" :title="row.user?.email || ''">{{ row.user?.email || '—' }}</div><div class="text-xs text-gray-400">#{{ row.user?.id || row.user_id || '—' }}<span v-if="row.user?.deleted_at" class="ml-1 text-rose-500">{{ t('admin.requestRecords.deleted') }}</span></div></div></template>
          <template #cell-api_key="{ row }"><div class="min-w-[140px] max-w-[220px]"><div class="truncate text-gray-900 dark:text-white" :title="row.api_key?.name || ''">{{ row.api_key?.name || '—' }}</div><div class="text-xs text-gray-400">#{{ row.api_key?.id || row.api_key_id || '—' }}</div></div></template>
          <template #cell-account="{ row }"><span class="text-gray-700 dark:text-gray-300">{{ row.account?.name || (row.account_id ? `#${row.account_id}` : '—') }}</span></template>
          <template #cell-group="{ row }"><span v-if="row.group?.name" class="inline-flex items-center rounded bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200">{{ row.group.name }}</span><span v-else class="text-gray-400">{{ row.group_id ? `#${row.group_id}` : '—' }}</span></template>
          <template #cell-model="{ row }"><span class="block max-w-[220px] truncate font-medium text-gray-900 dark:text-white" :title="displayModel(row)">{{ displayModel(row) || '—' }}</span></template>
          <template #cell-endpoint="{ row }"><div class="max-w-[320px]"><div class="truncate font-mono text-xs text-gray-700 dark:text-gray-300" :title="row.path">{{ row.method }} {{ row.path }}</div><div v-if="row.query_string" class="truncate text-xs text-gray-400" :title="row.query_string">?{{ row.query_string }}</div></div></template>
          <template #cell-type="{ row }"><span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium" :class="row.stream ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'">{{ row.stream ? t('admin.requestRecords.stream') : t('admin.requestRecords.sync') }}</span></template>
          <template #cell-status="{ row }"><span class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium" :class="statusClass(row.response_status)"><span class="h-1.5 w-1.5 rounded-full" :class="row.response_status >= 400 ? 'bg-rose-500' : 'bg-emerald-500'"></span>{{ row.response_status }}</span></template>
          <template #cell-duration_ms="{ value }"><span class="whitespace-nowrap text-gray-600 dark:text-gray-300">{{ value }} ms</span></template>
          <template #cell-request_id="{ value }"><span class="block max-w-[170px] truncate font-mono text-xs text-gray-500" :title="value">{{ value || '—' }}</span></template>
          <template #cell-client_request_id="{ value }"><span class="block max-w-[170px] truncate font-mono text-xs text-gray-500" :title="value">{{ value || '—' }}</span></template>
          <template #cell-user_agent="{ value }"><span class="block max-w-[240px] truncate text-xs text-gray-500" :title="value">{{ value || '—' }}</span></template>
          <template #cell-ip_address="{ value }"><span class="whitespace-nowrap font-mono text-xs text-gray-500">{{ value || '—' }}</span></template>
          <template #cell-created_at="{ value }"><span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span></template>
          <template #empty><div class="p-8 text-center text-sm text-gray-500">{{ t('common.noData') }}</div></template>
        </DataTable>

        <Pagination v-if="!conversationMode && total > 0" :total="total" :page="page" :page-size="pageSize" @update:page="onPageChange" @update:page-size="onPageSizeChange" />

        <div v-else class="min-h-[360px] bg-gray-50/60 p-4 dark:bg-dark-900/30 sm:p-6">
          <div v-if="conversationLoading" class="flex min-h-[300px] items-center justify-center text-sm text-gray-500">{{ t('admin.requestRecords.conversationLoading') }}</div>
          <div v-else-if="conversationRecords.length === 0" class="flex min-h-[300px] items-center justify-center text-sm text-gray-500">{{ t('admin.requestRecords.conversationEmpty') }}</div>
          <div v-else class="mx-auto max-w-5xl space-y-6">
            <div class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-primary-100 bg-primary-50 px-4 py-3 text-sm dark:border-primary-900/50 dark:bg-primary-950/30"><div><span class="font-medium text-primary-800 dark:text-primary-200">{{ t('admin.requestRecords.conversationView') }}</span><span class="ml-2 text-primary-600/80 dark:text-primary-300/80">{{ apiKeyKeyword || `#${filters.api_key_id}` }}</span></div><span class="text-primary-600/80 dark:text-primary-300/80">{{ conversationRecords.length }}<span v-if="total > conversationRecords.length"> / {{ total }}</span> {{ t('admin.requestRecords.records') }}</span></div>
            <div v-for="record in conversationRecords" :key="record.id" class="space-y-3">
              <div class="flex items-center justify-center gap-2 text-xs text-gray-400"><span>{{ formatTime(record.created_at) }}</span><span>·</span><span>{{ displayModel(record) || '—' }}</span><span>·</span><span>{{ record.method }} {{ record.path }}</span><button type="button" class="text-primary-600 hover:underline dark:text-primary-400" @click="openDetail(record)">{{ t('admin.requestRecords.openDetail') }}</button></div>
              <div class="flex justify-end"><div class="max-w-[92%] rounded-2xl rounded-tr-md bg-primary-600 px-4 py-3 text-white shadow-sm dark:bg-primary-700"><div class="mb-1 text-xs font-semibold uppercase tracking-wide text-primary-100">{{ t('admin.requestRecords.turnRequest') }}</div><pre class="max-h-80 overflow-auto whitespace-pre-wrap break-words text-sm leading-6">{{ conversationRequest(record) }}</pre></div></div>
              <div class="flex justify-start"><div class="max-w-[92%] rounded-2xl rounded-tl-md border border-gray-200 bg-white px-4 py-3 text-gray-800 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"><div class="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"><span>{{ t('admin.requestRecords.turnResponse') }}</span><span :class="record.response_status >= 400 ? 'text-rose-500' : 'text-emerald-500'">{{ record.response_status }}</span></div><pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-6">{{ conversationResponse(record) }}</pre></div></div>
            </div>
          </div>
        </div>
        <Pagination v-if="conversationMode && total > 0" :total="total" :page="page" :page-size="pageSize" @update:page="onPageChange" @update:page-size="onPageSizeChange" />
      </div>

      <BaseDialog v-if="selected" :show="true" :title="detailTitle" width="full" close-on-click-outside @close="selected = null">
        <div class="space-y-5">
        <div class="mb-5 grid gap-3 rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800 sm:grid-cols-2 lg:grid-cols-4"><div><span class="text-gray-500">{{ t('admin.requestRecords.user') }}</span><div class="font-medium">{{ selected.user?.email || selected.user_id || '—' }}</div></div><div><span class="text-gray-500">{{ t('admin.requestRecords.apiKey') }}</span><div class="font-medium">{{ selected.api_key?.name || selected.api_key_id || '—' }}</div></div><div><span class="text-gray-500">{{ t('admin.requestRecords.model') }}</span><div class="font-medium">{{ displayModel(selected) || '—' }}</div></div><div><span class="text-gray-500">{{ t('admin.requestRecords.status') }}</span><div class="font-medium" :class="selected.response_status >= 400 ? 'text-rose-600' : 'text-emerald-600'">{{ selected.response_status }} · {{ selected.duration_ms }} ms</div></div></div>
        <div class="grid gap-4 lg:grid-cols-2"><div><h3 class="mb-2 text-sm font-medium">{{ t('admin.requestRecords.request') }}</h3><pre class="mb-2 max-h-40 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ formatHeaders(selected.request_headers) }}</pre><pre class="max-h-96 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ formatPayload(selected.request_body) }}</pre></div><div><h3 class="mb-2 text-sm font-medium">{{ t('admin.requestRecords.response') }}</h3><pre class="mb-2 max-h-40 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ formatHeaders(selected.response_headers) }}</pre><pre class="max-h-96 overflow-auto rounded bg-gray-50 p-3 text-xs dark:bg-dark-800">{{ formatPayload(selected.response_body) }}</pre></div></div>
        </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { adminUsageAPI, type SimpleApiKey, type SimpleUser } from '@/api/admin/usage'
import requestRecordsAPI, { type RequestRecord, type RequestRecordQuery } from '@/api/admin/requestRecords'

const { t } = useI18n()
const records = ref<RequestRecord[]>([])
const selected = ref<RequestRecord | null>(null)
const loading = ref(false)
const exporting = ref(false)
const page = ref(1)
const pageSize = ref(50)
const total = ref(0)
const filters = reactive<RequestRecordQuery & { start_date: string; end_date: string }>({ path: '', method: '', model: '', status_code: undefined, user_id: undefined, api_key_id: undefined, start_date: '', end_date: '' })
const userSearchRef = ref<HTMLElement | null>(null)
const apiKeySearchRef = ref<HTMLElement | null>(null)
const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const showUserDropdown = ref(false)
const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const showApiKeyDropdown = ref(false)
const conversationMode = ref(false)
const conversationLoading = ref(false)
const conversationRecords = ref<RequestRecord[]>([])
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

const allColumns = computed(() => [
  { key: 'user', label: t('admin.requestRecords.user') }, { key: 'api_key', label: t('admin.requestRecords.apiKey') }, { key: 'account', label: t('admin.requestRecords.account') }, { key: 'group', label: t('admin.requestRecords.group') }, { key: 'model', label: t('admin.requestRecords.model') }, { key: 'endpoint', label: t('admin.requestRecords.endpoint') }, { key: 'type', label: t('admin.requestRecords.type') }, { key: 'status', label: t('admin.requestRecords.status') }, { key: 'duration_ms', label: t('admin.requestRecords.duration') }, { key: 'request_id', label: t('admin.requestRecords.requestId') }, { key: 'client_request_id', label: t('admin.requestRecords.clientRequestId') }, { key: 'user_agent', label: t('admin.requestRecords.userAgent') }, { key: 'ip_address', label: t('admin.requestRecords.ipAddress') }, { key: 'created_at', label: t('admin.requestRecords.created') }
])
const detailTitle = computed(() => selected.value ? `${t('admin.requestRecords.detail')} #${selected.value.id}` : t('admin.requestRecords.detail'))
const modelOptions = computed<SelectOption[]>(() => {
  const models = new Set(records.value.map((record) => displayModel(record)).filter(Boolean))
  if (filters.model) models.add(filters.model)
  return [{ value: null, label: t('admin.requestRecords.allModels') }, ...Array.from(models).sort().map((model) => ({ value: model, label: model }))]
})
const methodOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.requestRecords.allMethods') },
  ...['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'].map((method) => ({ value: method, label: method }))
])
const statusOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.requestRecords.allStatuses') },
  ...[200, 201, 202, 204, 400, 401, 403, 404, 408, 409, 422, 429, 500, 502, 503, 504].map((status) => ({ value: status, label: String(status) }))
])
const alwaysVisible = ['user', 'api_key', 'model', 'endpoint', 'status', 'created_at']
const toggleableColumns = computed(() => allColumns.value.filter((column) => !alwaysVisible.includes(column.key)))
const hiddenColumns = reactive(new Set<string>())
const visibleColumns = computed(() => allColumns.value.filter((column) => alwaysVisible.includes(column.key) || !hiddenColumns.has(column.key)))
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)
const hiddenColumnsKey = 'request-records-hidden-columns'
const isColumnVisible = (key: string) => !hiddenColumns.has(key)
const toggleColumn = (key: string) => { if (hiddenColumns.has(key)) hiddenColumns.delete(key); else hiddenColumns.add(key); localStorage.setItem(hiddenColumnsKey, JSON.stringify([...hiddenColumns])) }
const handleColumnClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) showColumnDropdown.value = false
  if (userSearchRef.value && !userSearchRef.value.contains(target)) showUserDropdown.value = false
  if (apiKeySearchRef.value && !apiKeySearchRef.value.contains(target)) showApiKeyDropdown.value = false
}

let userSearchSequence = 0
const clearPendingUserSearch = () => {
  if (userSearchTimeout) clearTimeout(userSearchTimeout)
  userSearchTimeout = null
  userSearchSequence += 1
}
const debounceUserSearch = () => {
  clearPendingUserSearch()
  const query = userKeyword.value.trim()
  if (!query) {
    userResults.value = []
    return
  }
  const sequence = userSearchSequence
  userSearchTimeout = setTimeout(async () => {
    userSearchTimeout = null
    try {
      const results = await adminUsageAPI.searchUsers(query)
      if (sequence === userSearchSequence) userResults.value = results.sort((a, b) => Number(a.deleted) - Number(b.deleted))
    } catch {
      if (sequence === userSearchSequence) userResults.value = []
    }
  }, 300)
}
const clearApiKeyState = () => {
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  apiKeySearchTimeout = null
  apiKeyKeyword.value = ''
  apiKeyResults.value = []
  showApiKeyDropdown.value = false
  filters.api_key_id = undefined
  conversationMode.value = false
  conversationRecords.value = []
}
const debounceApiKeySearch = () => {
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  apiKeySearchTimeout = setTimeout(async () => {
    try {
      apiKeyResults.value = await adminUsageAPI.searchApiKeys(filters.user_id, apiKeyKeyword.value.trim())
    } catch {
      apiKeyResults.value = []
    }
  }, 300)
}
const onApiKeyFocus = () => {
  showApiKeyDropdown.value = true
  if (apiKeyResults.value.length === 0) debounceApiKeySearch()
}
const clearUserState = () => {
  clearPendingUserSearch()
  userKeyword.value = ''
  userResults.value = []
  showUserDropdown.value = false
  filters.user_id = undefined
  clearApiKeyState()
}
const selectUser = async (user: SimpleUser) => {
  clearPendingUserSearch()
  userKeyword.value = user.email
  userResults.value = []
  showUserDropdown.value = false
  filters.user_id = user.id
  clearApiKeyState()
  try {
    apiKeyResults.value = await adminUsageAPI.searchApiKeys(user.id, '')
  } catch {
    apiKeyResults.value = []
  }
  search()
}
const clearUser = () => {
  clearUserState()
  search()
}
const selectApiKey = (key: SimpleApiKey) => {
  apiKeyKeyword.value = key.name || String(key.id)
  apiKeyResults.value = []
  showApiKeyDropdown.value = false
  filters.api_key_id = key.id
  search()
}
const clearApiKey = () => {
  clearApiKeyState()
  search()
}

async function loadConversationPage(resetPage = false) {
  const apiKeyID = filters.api_key_id
  if (!apiKeyID) return
  if (resetPage) page.value = 1
  conversationMode.value = true
  conversationLoading.value = true
  try {
    const result = await requestRecordsAPI.list({ ...buildQuery(), api_key_id: apiKeyID, page: page.value, page_size: pageSize.value })
    conversationRecords.value = [...result.items].sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
    total.value = result.total
  } finally {
    conversationLoading.value = false
  }
}
async function openConversation() {
  await loadConversationPage(true)
}
function closeConversation() {
  conversationMode.value = false
}
function decodeConversationBody(value?: string): unknown {
  if (!value) return ''
  let decoded = value
  try {
    const binary = atob(value)
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0))
    decoded = new TextDecoder().decode(bytes)
  } catch {
    // Keep legacy/plain-text bodies unchanged.
  }
  try {
    return JSON.parse(decoded)
  } catch {
    return decoded
  }
}
function conversationContent(value: unknown): string {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (Array.isArray(value)) return value.map((item) => conversationContent(item)).filter(Boolean).join('\n')
  if (typeof value === 'object') {
    const item = value as Record<string, unknown>
    if (item.text != null) return conversationContent(item.text)
    if (item.content != null) return conversationContent(item.content)
    if (item.output_text != null) return conversationContent(item.output_text)
    if (item.parts != null) return conversationContent(item.parts)
  }
  return JSON.stringify(value, null, 2)
}
function prettyConversationBody(value: unknown): string {
  if (typeof value === 'string') return value || '—'
  if (value == null) return '—'
  return JSON.stringify(value, null, 2)
}
function conversationRequest(record: RequestRecord): string {
  const body = decodeConversationBody(record.request_body)
  if (!body || typeof body !== 'object') return prettyConversationBody(body)
  const payload = body as Record<string, unknown>
  const messages = payload.messages ?? payload.contents
  if (Array.isArray(messages) && messages.length > 0) {
    return messages.map((message) => {
      if (!message || typeof message !== 'object') return conversationContent(message)
      const item = message as Record<string, unknown>
      const role = typeof item.role === 'string' ? `${item.role}: ` : ''
      return `${role}${conversationContent(item.content ?? item.parts ?? item.text)}`
    }).join('\n\n')
  }
  if (payload.input != null) return conversationContent(payload.input)
  if (payload.prompt != null) return conversationContent(payload.prompt)
  return prettyConversationBody(body)
}
function conversationResponse(record: RequestRecord): string {
  const body = decodeConversationBody(record.response_body)
  if (!body || typeof body !== 'object') return prettyConversationBody(body)
  const payload = body as Record<string, unknown>
  if (Array.isArray(payload.choices) && payload.choices.length > 0) {
    return payload.choices.map((choice) => {
      if (!choice || typeof choice !== 'object') return conversationContent(choice)
      const item = choice as Record<string, unknown>
      const message = item.message ?? item.delta ?? item.content ?? item.text
      return conversationContent(message)
    }).filter(Boolean).join('\n\n')
  }
  if (payload.output_text != null) return conversationContent(payload.output_text)
  if (payload.output != null) return conversationContent(payload.output)
  if (payload.content != null) return conversationContent(payload.content)
  if (payload.text != null) return conversationContent(payload.text)
  return prettyConversationBody(body)
}

const toRFC3339 = (date: string, endOfDay = false) => date ? new Date(`${date}T${endOfDay ? '23:59:59.999' : '00:00:00'}`).toISOString() : undefined
const buildQuery = (): RequestRecordQuery => ({ page: page.value, page_size: pageSize.value, path: filters.path || undefined, method: filters.method || undefined, model: filters.model || undefined, status_code: filters.status_code || undefined, user_id: filters.user_id || undefined, api_key_id: filters.api_key_id || undefined, start_time: toRFC3339(filters.start_date), end_time: toRFC3339(filters.end_date, true) })

async function load() { loading.value = true; try { const result = await requestRecordsAPI.list(buildQuery()); records.value = result.items; total.value = result.total } finally { loading.value = false } }
function search() { page.value = 1; if (conversationMode.value) void openConversation(); else load() }
function reset() { filters.path = ''; filters.method = ''; filters.model = ''; filters.status_code = undefined; filters.start_date = ''; filters.end_date = ''; clearUserState(); search() }
function onPageChange(nextPage: number) { page.value = nextPage; if (conversationMode.value) void loadConversationPage(); else load() }
function onPageSizeChange(nextPageSize: number) { pageSize.value = nextPageSize; page.value = 1; if (conversationMode.value) void loadConversationPage(); else load() }
function openDetail(record: RequestRecord) { selected.value = record }

async function download() { exporting.value = true; try { const query = buildQuery(); delete query.page; delete query.page_size; const blob = await requestRecordsAPI.exportCSV(query); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'request-records.csv'; anchor.click(); URL.revokeObjectURL(url) } finally { exporting.value = false } }
function formatTime(value: string) { return new Date(value).toLocaleString() }
function formatPayload(value?: string) { if (!value) return '—'; let decoded = value; try { decoded = atob(value) } catch { /* legacy/plain response */ } try { return JSON.stringify(JSON.parse(decoded), null, 2) } catch { return decoded } }
function formatHeaders(value: Record<string, string[]>) { return JSON.stringify(value || {}, null, 2) }
function displayModel(record: RequestRecord) { if (record.model) return record.model; const raw = record.request_body ? (() => { try { return atob(record.request_body) } catch { return record.request_body || '' } })() : ''; try { const body = JSON.parse(raw); return body.model || body.session?.model || '' } catch { return '' } }
function statusClass(status: number) { if (status >= 500) return 'bg-rose-100 text-rose-800 dark:bg-rose-900 dark:text-rose-200'; if (status >= 400) return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'; return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200' }

onMounted(() => { try { const saved = JSON.parse(localStorage.getItem(hiddenColumnsKey) || '[]') as string[]; saved.forEach((key) => hiddenColumns.add(key)) } catch { /* ignore malformed local preferences */ }; document.addEventListener('click', handleColumnClickOutside); load() })
onUnmounted(() => { clearPendingUserSearch(); if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout); document.removeEventListener('click', handleColumnClickOutside) })
</script>
