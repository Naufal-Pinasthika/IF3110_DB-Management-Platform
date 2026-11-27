import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, NavLink, Navigate, Outlet, useLocation, useMatch, useNavigate, useParams, useSearchParams } from 'react-router'
import { accessLabel, api, databaseURL, encode } from '../api'
import { resetSession, useSession } from '../session'
import { useUI } from '../ui-context'
import { useDatabaseQuery, useSchemas, useTables } from '../database-hooks'
import { ActionButton, ErrorState, Icon, Loading } from './ui'

// Show matching tables below a schema in the left navigation
function SchemaBranch({ databaseId, name, selected, filter }: {
  databaseId: string
  name: string
  selected: boolean
  filter: string
}) {
  const [open, setOpen] = useState(selected)
  const navigate = useNavigate()
  const location = useLocation()
  const tables = useTables(databaseId, name, open || selected || Boolean(filter))
  const matching = name.toLowerCase().includes(filter)

  // Match tables by their full name when the schema name does not match
  const visibleTables = (tables.data || []).filter(
    table => matching || `${name}.${table.name}`.toLowerCase().includes(filter),
  )
  const hidden = Boolean(filter) && !matching && tables.isSuccess && !visibleTables.length

  return (
    <details
      hidden={hidden}
      open={filter ? !hidden : open || selected}
      onToggle={event => setOpen(event.currentTarget.open)}
    >
      <summary
        aria-current={selected ? 'true' : undefined}
        onClick={() => {
          if (location.pathname === databaseURL(databaseId)) {
            void navigate(`${databaseURL(databaseId)}?${new URLSearchParams({ schema: name })}`)
          }
        }}
      >
        {name}
      </summary>
      <div className="object-links">
        {tables.isPending && (open || selected || filter) && <Loading>Loading tables…</Loading>}
        <ErrorState error={tables.error} retry={() => tables.refetch()} />
        {visibleTables.map(table => (
          <NavLink
            key={table.name}
            to={`${databaseURL(databaseId)}/schemas/${encode(name)}/tables/${encode(table.name)}`}
            aria-label={`${name}.${table.name}`}
          >
            <Icon name="table" />
            {table.name}
          </NavLink>
        ))}
        {tables.isSuccess && !visibleTables.length && (
          <Loading>{filter ? 'No matching tables.' : 'No tables'}</Loading>
        )}
      </div>
    </details>
  )
}

// Load schemas and keep the selected table visible in the object tree
function ObjectExplorer({ databaseId }: { databaseId: string }) {
  const [filter, setFilter] = useState('')
  const schemas = useSchemas(databaseId)
  const tableRoute = useMatch('/databases/:dbId/schemas/:schema/tables/:table')
  const [search] = useSearchParams()

  // Prefer the table route then the query value then the first available schema
  const selected = tableRoute?.params.schema || search.get('schema') || schemas.data?.[0]?.name

  return (
    <details className="object-explorer" open>
      <summary>Objects</summary>
      <label className="object-search">
        <span className="sr-only">Find schema or table</span>
        <input
          type="search"
          placeholder="Find schema or table…"
          value={filter}
          onChange={event => setFilter(event.target.value)}
        />
      </label>
      <div className="object-tree">
        {schemas.isPending && <Loading>Loading schemas…</Loading>}
        <ErrorState error={schemas.error} retry={() => schemas.refetch()} />
        {(schemas.data || []).map(schema => (
          <SchemaBranch
            key={schema.name}
            databaseId={databaseId}
            name={schema.name}
            selected={selected === schema.name}
            filter={filter.trim().toLowerCase()}
          />
        ))}
        {schemas.isSuccess && !schemas.data?.length && <Loading>No schemas available.</Loading>}
      </div>
    </details>
  )
}

// Send guests to login and keep the requested safe page for after login
export function RequireSession() {
  const session = useSession()
  const location = useLocation()

  if (session.isPending) return <Loading>Loading your workspace…</Loading>
  if (session.error) {
    return (
      <main className="content">
        <ErrorState error={session.error} retry={() => session.refetch()} />
      </main>
    )
  }

  if (!session.data) {
    // Keep sensitive pages out of the saved return path
    const isSensitive = location.pathname.startsWith('/admin') ||
      (location.pathname.startsWith('/databases/') && location.pathname !== '/databases')
    const search = isSensitive
      ? ''
      : `?${new URLSearchParams({ next: location.pathname + location.search + location.hash })}`

    return <Navigate to={`/login${search}`} replace />
  }

  return <Outlet />
}

// Keep platform management routes limited to platform admins
export function RequireAdmin() {
  const { data: user } = useSession()

  if (!user?.is_platform_admin) return <Navigate to="/databases" replace />
  return <Outlet />
}

// Load database details before rendering its child routes
export function DatabaseLayout() {
  const { dbId = '' } = useParams()
  const database = useDatabaseQuery(dbId)

  if (database.isPending) return <Loading>Loading database…</Loading>
  if (database.error) {
    return (
      <main className="content">
        <ErrorState error={database.error} />
        <div style={{ marginTop: '1rem' }}>
          <Link to="/databases" className="button primary">
            Back to your databases
          </Link>
        </div>
      </main>
    )
  }

  if (!database.data) return null
  return <Outlet context={database.data} key={dbId} />
}

// Render navigation and the active page around every signed in route
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
  const route = location.pathname + location.search
  const [previousRoute, setPreviousRoute] = useState(route)

  // Close the mobile drawer whenever navigation changes
  if (previousRoute !== route) {
    setPreviousRoute(route)
    setMobileOpen(false)
  }

  // Move focus to the main content after each route change
  useEffect(() => {
    main.current?.focus({ preventScroll: true })
  }, [location.pathname, location.search])

  // Start each page with its navigation and connection context visible.
  useEffect(() => {
    window.scrollTo({ top: 0, left: 0, behavior: 'instant' })
  }, [location.pathname])

  // Keep the mobile drawer keyboard and screen reader state in sync
  useEffect(() => {
    document.body.classList.toggle('navigation-open', mobileOpen)

    const close = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMobileOpen(false)
        menuButton.current?.focus()
      }
    }

    const media = window.matchMedia('(max-width: 760px)')
    const resized = () => {
      if (!media.matches) setMobileOpen(false)
    }

    window.addEventListener('keydown', close)
    media.addEventListener('change', resized)

    if (mobileOpen) sidebar.current?.querySelector<HTMLAnchorElement>('a')?.focus()

    return () => {
      window.removeEventListener('keydown', close)
      media.removeEventListener('change', resized)
      document.body.classList.remove('navigation-open')
    }
  }, [mobileOpen])

  return (
    <>
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <div className={`app-shell${dbId ? ' database-shell' : ''}`}>
        <aside ref={sidebar} id="sidebar" className="sidebar" aria-label="Workspace navigation">
          <Link className="brand" to="/databases">
            <Icon name="database" />
            <span>Database Platform</span>
          </Link>
          <nav className="main-nav" aria-label="Main">
            <NavLink to="/databases" end>
              <Icon name="database" />
              Databases
            </NavLink>
            {dbId && (
              <>
                <Link
                  to={base}
                  aria-current={
                    location.pathname === base || location.pathname.includes('/schemas/')
                      ? 'page'
                      : undefined
                  }
                >
                  <Icon name="table" />
                  Explorer
                </Link>
                <NavLink to={`${base}/query`}>
                  <Icon name="query" />
                  SQL query
                </NavLink>
                <NavLink to={`${base}/migrations`}>
                  <Icon name="history" />
                  Migrations
                </NavLink>
              </>
            )}
          </nav>
          {dbId && database.data && <ObjectExplorer databaseId={dbId} key={dbId} />}
          {user?.is_platform_admin && (
            <div className="admin-navigation">
              <nav className="main-nav" aria-label="Administration">
                <NavLink to="/admin/users">
                  <Icon name="users" />
                  Users
                </NavLink>
                <NavLink to="/admin/groups">
                  <Icon name="users" />
                  Groups &amp; access
                </NavLink>
                <NavLink to="/admin/databases">
                  <Icon name="connect" />
                  Connections
                </NavLink>
              </nav>
            </div>
          )}
        </aside>
        <div className="workspace">
          <header className={`topbar${dbId ? ' database-topbar' : ''}`}>
            <button
              ref={menuButton}
              type="button"
              className="button quiet mobile-menu-button"
              aria-expanded={mobileOpen}
              aria-controls="sidebar"
              onClick={() => setMobileOpen(!mobileOpen)}
            >
              {mobileOpen ? 'Close' : 'Menu'}
            </button>
            {dbId ? (
              <dl className="database-navbar-info" aria-label="Current database connection">
                <div className="navbar-info-field">
                  <dt>Connection</dt>
                  <dd>
                    <Link to="/databases" title={database.data?.label}>
                      {database.data ? database.data.label || 'Unlabeled connection' : database.error ? 'Unavailable' : 'Loading…'}
                    </Link>
                  </dd>
                </div>
                <div className="navbar-info-field">
                  <dt>Database</dt>
                  <dd className="mono" title={database.data?.database_name}>
                    {database.data?.database_name || (database.error ? 'Unavailable' : 'Loading…')}
                  </dd>
                </div>
                <div className="navbar-info-field">
                  <dt>Access</dt>
                  <dd>
                    <span className={`navbar-access${database.data?.effective_access === 'VIEW_EDIT' ? ' can-migrate' : ''}`}>
                      {database.data
                        ? accessLabel(database.data.effective_access)
                        : database.error
                          ? 'Unavailable'
                          : 'Checking…'}
                    </span>
                  </dd>
                </div>
                <div className="navbar-info-field">
                  <dt>SQL provider</dt>
                  <dd>
                    {database.data
                      ? database.data.engine === 'mysql' ? 'MySQL' : 'PostgreSQL'
                      : database.error ? 'Unavailable' : 'Loading…'}
                  </dd>
                </div>
              </dl>
            ) : null}
            <div className="account" inert={mobileOpen}>
              <span className="avatar" aria-hidden="true">
                {user?.username.slice(0, 1).toUpperCase()}
              </span>
              <span id="account-name">{user?.username}</span>
              <ActionButton
                id="logout"
                className="button quiet small"
                onAction={async () => {
                  // Let editors protect unsaved work before ending the session
                  const event = new CustomEvent('request-signout', { cancelable: true })
                  if (
                    !window.dispatchEvent(event) &&
                    !await confirm({
                      title: 'Leave without saving?',
                      description: 'Your unsaved changes will be lost.',
                      label: 'Sign out',
                    })
                  ) return

                  await api('/api/v1/auth/logout', { method: 'POST' })
                  window.dispatchEvent(new Event('discard-changes'))
                  await navigate('/login', { replace: true })
                  await resetSession(client, null)
                }}
              >
                Sign out
              </ActionButton>
            </div>
          </header>
          <main ref={main} id="main" className="content" tabIndex={-1} inert={mobileOpen}>
            <div id="page-content">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </>
  )
}
