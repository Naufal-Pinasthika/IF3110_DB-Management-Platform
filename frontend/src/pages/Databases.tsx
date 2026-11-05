import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { accessDescription, accessLabel, api, databaseURL } from '../api'
import { useSession } from '../providers'
import type { Database } from '../types'
import ConnectionDialog from '../components/ConnectionDialog'
import { ActionButton, EmptyRow, ErrorState, PageHeading } from '../components/ui'

export default function Databases() {
  const { data: user } = useSession()
  const query = useQuery({ queryKey: ['databases'], queryFn: ({ signal }) => api<Database[]>('/api/v1/databases', { signal }) })
  const [filter, setFilter] = useState('')
  const [adding, setAdding] = useState(false)
  const databases = (query.data || []).filter(database => `${database.label} ${database.database_name}`.toLowerCase().includes(filter.toLowerCase()))
  return <>
    <PageHeading title="Databases">{user?.is_platform_admin && <button className="button primary" onClick={() => setAdding(true)}>Add database</button>}</PageHeading>
    <ErrorState error={query.error} retry={() => query.refetch()} />
    <section className="database-index" aria-label="Database connections"><div className="list-toolbar">
      <label className="search-field"><span className="sr-only">Filter databases</span><input type="search" id="database-filter" placeholder="Search databases by label or name…" value={filter} onChange={event => setFilter(event.target.value)} /></label>
      <span className="record-count">{query.isPending ? 'Loading…' : `${databases.length}${filter ? ` of ${query.data?.length || 0}` : ''} connection${databases.length === 1 ? '' : 's'}`}</span>
      <ActionButton onAction={() => query.refetch()} disabled={query.isFetching}>Refresh</ActionButton>
    </div><div className="table-scroll database-grid"><table><thead><tr><th>Connection</th><th>Database</th><th>Access</th><th><span className="sr-only">Open</span></th></tr></thead><tbody>
      {query.isPending ? <EmptyRow columns={4}>Loading databases…</EmptyRow> : !databases.length ? <EmptyRow columns={4}>{filter ? 'No databases match your filter.' : user?.is_platform_admin ? 'No enabled databases available. Add a database connection in Administration to get started.' : 'No databases are assigned to your groups. Ask an administrator for access.'}</EmptyRow> : databases.map(database => <tr key={database.id}>
        <td><Link to={databaseURL(database.id)}>{database.label}</Link></td><td className="mono">{database.database_name}</td>
        <td><div title={accessDescription(database.effective_access)}><span className="permission-name">{accessLabel(database.effective_access)}</span><span className="permission-detail">{database.effective_access === 'VIEW_EDIT' ? 'SELECT - create & execute migrations' : 'SELECT - schema - history'}</span></div></td>
        <td><Link className="button small open-database" to={databaseURL(database.id)}>Open</Link></td>
      </tr>)}
    </tbody></table></div></section>
    {adding && <ConnectionDialog onClose={() => setAdding(false)} />}
  </>
}
