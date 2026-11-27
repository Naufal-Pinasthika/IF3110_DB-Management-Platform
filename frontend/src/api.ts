// Keep server status and code next to the message for page error handling
export class APIError extends Error {
  status: number
  code: string

  constructor(message: string, status = 0, code = '') {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
  }
}

// Build paths with encoded ids before using them in routes
export const encode = encodeURIComponent
export const databaseURL = (id: string) => `/databases/${encode(id)}`
export const databaseAPI = (id: string) => `/api/v1${databaseURL(id)}`

// Read the cookie token required by state changing requests
function csrfToken() {
  const cookie = document.cookie
    .split(';')
    .map(value => value.trim())
    .find(value => value.startsWith('csrf_token='))

  try {
    return cookie ? decodeURIComponent(cookie.slice('csrf_token='.length)) : ''
  } catch {
    return ''
  }
}

// Send requests with cookies and return the data field from the shared response
export async function api<T>(
  path: string,
  options: { method?: string; data?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const { method = 'GET', data, signal } = options
  const headers: Record<string, string> = { Accept: 'application/json' }

  if (data !== undefined) headers['Content-Type'] = 'application/json'
  if (!['GET', 'HEAD'].includes(method)) headers['X-CSRF-Token'] = csrfToken()

  let response: Response

  try {
    response = await fetch(path, {
      method,
      headers,
      signal,
      credentials: 'same-origin',
      ...(data !== undefined ? { body: JSON.stringify(data) } : {}),
    })
  } catch (error) {
    // Keep user cancellation separate from a network failure
    if (signal?.aborted) throw error
    throw new APIError('Unable to reach the server. Check your connection and try again.')
  }

  // Ask the app to clear user data when another request finds an expired session
  if (response.status === 401 && !path.endsWith('/auth/login') && !path.endsWith('/auth/me')) {
    window.dispatchEvent(new Event('session-expired'))
  }

  // A no content response has no data body
  if (response.status === 204) return undefined as T

  let body

  try {
    body = await response.json()
  } catch {
    // Show a useful error when the server does not send JSON
    throw new APIError('The server returned an unexpected response. Please try again.', response.status)
  }

  if (!response.ok) {
    throw new APIError(
      body.error?.message || body.message || 'The request could not be completed.',
      response.status,
      body.error?.code || body.code || '',
    )
  }

  return body.data as T
}

// Convert thrown values into text that notices can show
export const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : 'Something went wrong. Please try again.'

// Convert database values into text that can be rendered
export function displayValue(value: unknown): string {
  if (value === null) return 'NULL'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value ?? '')
}

// Ignore missing or invalid dates so a page can still render
export function formatDate(value?: string | null) {
  if (!value) return '--'

  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '--'
    : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}

// Allow only local paths after login so redirects stay inside the app
export function returnPath(candidate: string | null) {
  try {
    const url = new URL(candidate || '/databases', window.location.origin)
    return url.origin === window.location.origin && url.pathname !== '/login'
      ? url.pathname + url.search + url.hash
      : '/databases'
  } catch {
    return '/databases'
  }
}

// Quote each part of a database name for its engine
export function qualifiedName(schema: string, table: string, engine = 'postgres') {
  const quote = engine === 'mysql' ? '`' : '"'
  return [schema, table]
    .map(name => quote + name.replaceAll(quote, quote + quote) + quote)
    .join('.')
}

// Explain permissions with the same words used in the access badge
export function accessLabel(level: string) {
  const labels: Record<string, string> = {
    VIEW_ONLY: 'Read only',
    VIEW_EDIT: 'Read + migrations',
    NONE: 'No access',
    DISABLED: 'Unavailable',
  }

  return labels[level] || level
}

export function accessDescription(level: string) {
  return level === 'VIEW_EDIT'
    ? 'Browse schema and data, run SELECT queries, review history, and create or execute migrations.'
    : level === 'VIEW_ONLY'
      ? 'Browse schema and data, run SELECT queries, and review migration history. Changes require migration access.'
      : 'Effective database permissions could not be verified.'
}
