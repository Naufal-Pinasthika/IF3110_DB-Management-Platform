import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, encode } from '../api'
import { useUI } from '../providers'
import type { Connection, ConnectionTest } from '../types'
import ConnectionDialog from '../components/ConnectionDialog'
import { ActionButton, Badge, EmptyRow, ErrorState, PageHeading } from '../components/ui'

export default function Connections() {
  const connections = useQuery({ queryKey: ['admin-connections'], queryFn: ({ signal }) => api<Connection[]>('/api/v1/admin/databases', { signal }) })
  const [editing, setEditing] = useState<string | null>(null)
  const client = useQueryClient()
  const { confirm, notify } = useUI()
  const change = async (connection: Connection, kind: 'disable' | 'enable' | 'delete') => {
    const options = {
      disable: { title: 'Disable connection?', description: `Users will no longer be able to access ${connection.label}. Migration history is preserved.`, label: 'Disable connection' },
      enable: { title: 'Re-enable connection?', description: `Users with group access will be able to connect to ${connection.label} again.`, label: 'Re-enable' },
      delete: { title: 'Delete disabled connection?', description: `Permanently remove ${connection.label}. Existing migration history is preserved and prevents deletion.`, label: 'Delete connection' },
    }
    if (!await confirm(options[kind])) return
    await api(`/api/v1/admin/databases/${encode(connection.id)}${kind === 'delete' ? '/delete' : ''}`, { method: kind === 'enable' ? 'PATCH' : 'DELETE', ...(kind === 'enable' ? { data: { is_enabled: true } } : {}) })
    notify(kind === 'disable' ? 'Connection disabled.' : kind === 'enable' ? 'Connection re-enabled.' : 'Connection deleted.')
    await client.invalidateQueries()
  }
  return <>
    <PageHeading title="Connections"><button className="button primary" onClick={() => setEditing('new')}>Add connection</button></PageHeading>
    <ErrorState error={connections.error} retry={() => connections.refetch()} />
    <section className="panel connections-panel"><div className="panel-toolbar"><div><h2>Registered connections</h2><p className="muted">Reader and migrator credentials are stored separately.</p></div><ActionButton onAction={() => connections.refetch()}>Refresh</ActionButton></div>
      <div className="table-scroll"><table><thead><tr><th>Connection</th><th>Database name</th><th>Status</th><th>Actions</th></tr></thead><tbody>
        {(connections.data || []).map(connection => <tr key={connection.id}>
          <td><strong>{connection.label}</strong></td><td className="mono">{connection.database_name}</td><td><Badge value={connection.is_enabled ? 'ENABLED' : 'DISABLED'} /></td>
          <td><div className="toolbar-actions"><button className="button small" onClick={() => setEditing(connection.id)}>Edit</button><ActionButton onAction={async () => {
            const result = await api<ConnectionTest>(`/api/v1/admin/databases/${encode(connection.id)}/test-connection`, { method: 'POST' })
            notify(result.connected ? 'Reader and migrator connections succeeded.' : `Connection test failed: ${result.message || 'Could not reach target database.'}`, result.connected ? 'success' : 'error')
          }}>Test</ActionButton>
            {connection.is_enabled ? <ActionButton className="button small danger" onAction={() => change(connection, 'disable')}>Disable</ActionButton> : <><ActionButton className="button small primary" onAction={() => change(connection, 'enable')}>Re-enable</ActionButton><ActionButton className="button small danger" onAction={() => change(connection, 'delete')}>Delete</ActionButton></>}
          </div></td>
        </tr>)}
        {!connections.data?.length && <EmptyRow columns={4}>{connections.isPending ? 'Loading connections…' : 'No connections yet. Add a database connection to get started.'}</EmptyRow>}
      </tbody></table></div>
    </section>
    {editing && <ConnectionDialog id={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} />}
  </>
}
