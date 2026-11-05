import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, NavLink, Navigate, Outlet, useLocation, useMatch, useNavigate, useOutletContext, useParams, useSearchParams } from 'react-router'
import { accessDescription, accessLabel, api, databaseAPI, databaseURL, encode } from '../api'
import { resetSession, useSession, useUI } from '../providers'
import type { Database, NamedObject } from '../types'
import { ActionButton, ErrorState, Icon, Loading } from './ui'

export function useDatabaseQuery(id: string) {
  return useQuery({ queryKey: ['database', id], queryFn: ({ signal }) => api<Database>(databaseAPI(id), { signal }), enabled: Boolean(id) })
}
export function useDatabase() { return useOutletContext<Database>() }
export function useSchemas(id: string) {
  return useQuery({ queryKey: ['schemas', id], queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas`, { signal }) })
}
export function useTables(id: string, schema: string, enabled = true) {
  return useQuery({ queryKey: ['tables', id, schema], queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas/${encode(schema)}/tables`, { signal }), enabled: Boolean(schema) && enabled })
}

function SchemaBranch({ databaseId, name, selected, filter }: { databaseId: string; name: string; selected: boolean; filter: string }) {
  const [open, setOpen] = useState(selected)
  const navigate = useNavigate()
  const location = useLocation()
  const tables = useTables(databaseId, name, open || selected || Boolean(filter))
  const matching = name.toLowerCase().includes(filter)
  const visibleTables = (tables.data || []).filter(table => matching || `${name}.${table.name}`.toLowerCase().includes(filter))
  const hidden = Boolean(filter) && !matching && tables.isSuccess && !visibleTables.length
  return <details hidden={hidden} open={filter ? !hidden : open || selected} onToggle={event => setOpen(event.currentTarget.open)}>
    <summary aria-current={selected ? 'true' : undefined} onClick={() => {
      if (location.pathname === databaseURL(databaseId)) void navigate(`${databaseURL(databaseId)}?${new URLSearchParams({ schema: name })}`)
    }}>{name}</summary>
    <div className="object-links">
      {tables.isPending && (open || selected || filter) && <Loading>Loading tables…</Loading>}
      <ErrorState error={tables.error} retry={() => tables.refetch()} />
      {visibleTables.map(table => <NavLink key={table.name} to={`${databaseURL(databaseId)}/schemas/${encode(name)}/tables/${encode(table.name)}`} aria-label={`${name}.${table.name}`}><Icon name="table" />{table.name}</NavLink>)}
      {tables.isSuccess && !visibleTables.length && <Loading>{filter ? 'No matching tables.' : 'No tables'}</Loading>}
    </div>
  </details>
}

function ObjectExplorer({ databaseId }: { databaseId: string }) {
  const [filter, setFilter] = useState('')
  const schemas = useSchemas(databaseId)
  const tableRoute = useMatch('/databases/:dbId/schemas/:schema/tables/:table')
  const [search] = useSearchParams()
  const selected = tableRoute?.params.schema || search.get('schema') || schemas.data?.[0]?.name
  return <details className="object-explorer" open>
    <summary>Objects</summary>
    <label className="object-search"><span className="sr-only">Find schema or table</span><input type="search" placeholder="Find schema or table…" value={filter} onChange={event => setFilter(event.target.value)} /></label>
    <div className="object-tree">
      {schemas.isPending && <Loading>Loading schemas…</Loading>}
      <ErrorState error={schemas.error} retry={() => schemas.refetch()} />
      {(schemas.data || []).map(schema => <SchemaBranch key={schema.name} databaseId={databaseId} name={schema.name} selected={selected === schema.name} filter={filter.trim().toLowerCase()} />)}
      {schemas.isSuccess && !schemas.data?.length && <Loading>No schemas available.</Loading>}
    </div>
  </details>
}

export function RequireSession() {
  const session = useSession()
  const location = useLocation()
  if (session.isPending) return <Loading>Loading your workspace…</Loading>
  if (session.error) return <main className="content"><ErrorState error={session.error} retry={() => session.refetch()} /></main>
  if (!session.data) return <Navigate to={`/login?${new URLSearchParams({ next: location.pathname + location.search + location.hash })}`} replace />
  return <Outlet />
}

export function RequireAdmin() {
  const { data: user } = useSession()
  if (!user?.is_platform_admin) return <div className="notice error" role="alert">Administrator access is required to manage this section.</div>
  return <Outlet />
}

export function DatabaseLayout() {
  const { dbId = '' } = useParams()
  const database = useDatabaseQuery(dbId)
  if (database.isPending) return <Loading>Loading database…</Loading>
  if (database.error) return <ErrorState error={database.error} retry={() => database.refetch()} />
  if (!database.data) return null
  return <Outlet context={database.data} key={dbId} />
}

export function Shell() {
  const client = useQueryClient()
  const { data: user } = useSession()
  const databaseRoute = useMatch('/databases/:dbId/*')
  const dbId = databaseRoute?.params.dbId || ''
  const database = useDatabaseQuery(dbId)
  const [mobileOpen, setMobileOpen] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()
  const { confirm } = useUI()
  const sidebar = useRef<HTMLElement>(null)
  const menuButton = useRef<HTMLButtonElement>(null)
  const main = useRef<HTMLElement>(null)
  const base = databaseURL(dbId)
  useEffect(() => {
    main.current?.focus()
    // Route changes also close the drawer after a guarded navigation succeeds.
    document.body.classList.remove('navigation-open')
    setMobileOpen(false)
  }, [location.pathname, location.search])
  useEffect(() => {
    document.body.classList.toggle('navigation-open', mobileOpen)
    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { setMobileOpen(false); menuButton.current?.focus() }
    }
    const media = window.matchMedia('(max-width: 760px)')
    const resized = () => { if (!media.matches) setMobileOpen(false) }
    window.addEventListener('keydown', close)
    media.addEventListener('change', resized)
    if (mobileOpen) sidebar.current?.querySelector<HTMLAnchorElement>('a')?.focus()
    return () => { window.removeEventListener('keydown', close); media.removeEventListener('change', resized); document.body.classList.remove('navigation-open') }
  }, [mobileOpen])
  return <>
    <a className="skip-link" href="#main">Skip to content</a>
    <div className="app-shell">
      <aside ref={sidebar} id="sidebar" className="sidebar" aria-label="Workspace navigation">
        <Link className="brand" to="/databases"><Icon name="database" /><span>Database Platform</span></Link>
        <nav className="main-nav" aria-label="Main">
          <NavLink to="/databases" end><Icon name="database" />Databases</NavLink>
          {dbId && <>
            <Link to={base} aria-current={location.pathname === base || location.pathname.includes('/schemas/') ? 'page' : undefined}><Icon name="table" />Explorer</Link>
            <NavLink to={`${base}/query`}><Icon name="query" />SQL query</NavLink>
            <NavLink to={`${base}/migrations`}><Icon name="history" />Migrations</NavLink>
          </>}
        </nav>
        {dbId && database.data && <ObjectExplorer databaseId={dbId} key={dbId} />}
        {user?.is_platform_admin && <details className="admin-navigation" open><summary>Administration</summary><nav className="main-nav" aria-label="Administration">
          <NavLink to="/admin/users"><Icon name="users" />Users</NavLink>
          <NavLink to="/admin/groups"><Icon name="users" />Groups &amp; access</NavLink>
          <NavLink to="/admin/databases"><Icon name="connect" />Connections</NavLink>
        </nav></details>}
      </aside>
      <div className="workspace">
        <header className="topbar">
          <button ref={menuButton} type="button" className="button quiet mobile-menu-button" aria-expanded={mobileOpen} aria-controls="sidebar" onClick={() => setMobileOpen(!mobileOpen)}>{mobileOpen ? 'Close' : 'Menu'}</button>
          {dbId ? <div className="connection-context">
            <Link to="/databases" className="switch-database">Databases /</Link>
            <div className="database-identity"><strong className="mono">{database.data?.database_name || 'Database'}</strong><span className="database-label">{database.data?.label}</span></div>
            <details className="access-details"><summary>{database.data ? accessLabel(database.data.effective_access) : database.error ? 'Access unavailable' : 'Checking access…'}</summary><div><p>{accessDescription(database.data?.effective_access || '')}</p><p className="muted">Connection has not been tested.</p></div></details>
            <span className="connection-status">{database.data?.engine === 'mysql' ? 'MySQL' : 'PostgreSQL'}</span>
          </div> : <span className="engine-label">PostgreSQL</span>}
          <div className="account" inert={mobileOpen}><span className="avatar" aria-hidden="true">{user?.username.slice(0, 1).toUpperCase()}</span><span id="account-name">{user?.username}</span>
            <ActionButton id="logout" className="button quiet small" onAction={async () => {
              // Give the active editor a chance to protect unsaved changes before ending the session.
              const event = new CustomEvent('request-signout', { cancelable: true })
              if (!window.dispatchEvent(event) && !await confirm({ title: 'Leave without saving?', description: 'Your unsaved changes will be lost.', label: 'Sign out' })) return
              await api('/api/v1/auth/logout', { method: 'POST' })
              window.dispatchEvent(new Event('discard-changes'))
              await resetSession(client, null)
              await navigate('/login', { replace: true })
            }}>Sign out</ActionButton>
          </div>
        </header>
        <main ref={main} id="main" className="content" tabIndex={-1} inert={mobileOpen}><div id="page-content"><Outlet /></div></main>
      </div>
    </div>
  </>
}
