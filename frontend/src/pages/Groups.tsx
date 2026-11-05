import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { accessLabel, api, encode } from '../api'
import { useUI } from '../providers'
import type { AccessLevel, AccessSnapshot, AdminUser, Connection, Grant, Group } from '../types'
import { ActionButton, Badge, EmptyRow, ErrorState, Loading, Modal, PageHeading, Tabs, useFormAction } from '../components/ui'

interface GroupData extends AccessSnapshot { groups: Group[]; users: AdminUser[]; databases: Connection[] }
type Editor = { kind: 'group'; group?: Group } | { kind: 'member'; group: Group } | { kind: 'grant'; group: Group; grant?: Grant }

function userAccess(data: GroupData, user: AdminUser, database: Connection, excludedGroup = '') {
  if (!user.is_active) return { level: 'DISABLED', sources: ['Inactive account'] }
  if (!database.is_enabled) return { level: 'DISABLED', sources: ['Disabled connection'] }
  if (user.is_platform_admin) return { level: 'VIEW_EDIT', sources: ['Platform administrator'] }
  const memberships = data.members.filter(member => member.user_id === user.id && member.group_id !== excludedGroup).map(member => member.group_id)
  const grants = data.grants.filter(grant => memberships.includes(grant.group_id) && grant.database_id === database.id)
  return {
    level: grants.some(grant => grant.access_level === 'VIEW_EDIT') ? 'VIEW_EDIT' : grants.length ? 'VIEW_ONLY' : 'NONE',
    sources: grants.map(grant => data.groups.find(group => group.id === grant.group_id)?.name || grant.group_id),
  }
}

export default function Groups() {
  const query = useQuery({ queryKey: ['admin-groups'], queryFn: async ({ signal }): Promise<GroupData> => {
    const [groups, users, databases, snapshot] = await Promise.all([
      api<Group[]>('/api/v1/admin/groups', { signal }), api<AdminUser[]>('/api/v1/admin/users', { signal }),
      api<Connection[]>('/api/v1/admin/databases', { signal }), api<AccessSnapshot>('/api/v1/admin/access', { signal }),
    ])
    return { groups: groups || [], users: users || [], databases: databases || [], members: snapshot.members || [], grants: snapshot.grants || [] }
  } })
  const client = useQueryClient()
  const { confirm, notify } = useUI()
  const [search, setSearch] = useSearchParams()
  const [tab, setTab] = useState('members')
  const [editor, setEditor] = useState<Editor | null>(null)
  const data = query.data
  const group = data?.groups.find(item => item.id === search.get('group')) || data?.groups[0]
  const members = data?.users.filter(user => data.members.some(member => member.group_id === group?.id && member.user_id === user.id)) || []
  const grants = data?.grants.filter(grant => grant.group_id === group?.id) || []
  const changed = async (message: string) => { notify(message); await client.invalidateQueries() }
  return <>
    <PageHeading title="Groups & access"><button className="button primary" onClick={() => setEditor({ kind: 'group' })}>New group</button><ActionButton onAction={() => query.refetch()}>Refresh</ActionButton></PageHeading>
    <ErrorState error={query.error} retry={() => query.refetch()} />
    {query.isPending && <Loading>Loading groups and access…</Loading>}
    {data && !data.groups.length && <div id="groups-empty" className="empty-state"><h2>Create your first group</h2><p>Organize users in a group, then assign access to its databases.</p><button className="button primary" onClick={() => setEditor({ kind: 'group' })}>Create group</button></div>}
    {data && group && <div className="groups-layout"><aside className="group-directory" aria-label="Groups"><h2>Groups</h2><div id="group-list">{data.groups.map(item => <button key={item.id} type="button" className="group-choice" aria-pressed={item.id === group.id} onClick={() => setSearch({ group: item.id })}>{item.name}</button>)}</div></aside>
      <section className="group-detail" aria-labelledby="selected-group-name">
        <div className="panel-toolbar"><div><h2 id="selected-group-name">{group.name}</h2><p>{members.length} member{members.length === 1 ? '' : 's'} - {grants.length} database{grants.length === 1 ? '' : 's'}</p></div><div className="toolbar-actions">
          <button type="button" className="button small" onClick={() => setEditor({ kind: 'group', group })}>Rename</button>
          <ActionButton className="button small danger" onAction={async () => {
            if (!await confirm({ title: `Delete ${group.name}?`, description: 'This removes the group, its memberships, and database grants. Access provided by other groups or administrator roles remains. Affected sessions will refresh.', label: 'Delete group' })) return
            await api(`/api/v1/admin/groups/${encode(group.id)}`, { method: 'DELETE' })
            await changed(`${group.name} deleted.`)
          }}>Delete group</ActionButton>
        </div></div>
        <Tabs selected={tab} onSelect={setTab} label="Group configuration" tabs={[{ id: 'members', label: 'Members' }, { id: 'grants', label: 'Database access' }, { id: 'effective', label: 'Effective access' }]} />
        <section role="tabpanel" aria-label="Members" hidden={tab !== 'members'}>
          <div className="section-toolbar"><p className="muted">Members can also inherit access from other groups.</p><button className="button primary small" onClick={() => setEditor({ kind: 'member', group })}>Add member</button></div>
          <div className="table-scroll"><table><thead><tr><th>Username</th><th>Platform role</th><th>Actions</th></tr></thead><tbody>{members.map(user => <tr key={user.id}><td><strong>{user.username}</strong></td><td>{user.is_active ? user.is_platform_admin ? 'Administrator' : 'User' : 'Inactive account'}</td><td><ActionButton className="button small danger" onAction={async () => {
            const retained = data.databases.filter(database => database.is_enabled).map(database => `${database.label}: ${accessLabel(userAccess(data, user, database, group.id).level)}`).join('; ')
            if (!await confirm({ title: `Remove ${user.username} from ${group.name}?`, description: `Access from other groups or administrator roles remains. After removal: ${retained || 'No enabled databases'}. The user's sessions will refresh.`, label: 'Remove member' })) return
            await api(`/api/v1/admin/groups/${encode(group.id)}/members/${encode(user.id)}`, { method: 'DELETE' })
            await changed(`${user.username} removed from ${group.name}.`)
          }}>Remove</ActionButton></td></tr>)}{!members.length && <EmptyRow columns={3}>No members yet. Add a user to this group.</EmptyRow>}</tbody></table></div>
        </section>
        <section role="tabpanel" aria-label="Database access" hidden={tab !== 'grants'}>
          <div className="section-toolbar"><p className="muted">Permissions assigned through this group.</p><button className="button primary small" onClick={() => setEditor({ kind: 'grant', group })}>Add database access</button></div>
          <div className="table-scroll"><table><thead><tr><th>Database</th><th>Access level</th><th>Actions</th></tr></thead><tbody>{grants.map(grant => {
            const database = data.databases.find(item => item.id === grant.database_id)
            return <tr key={grant.database_id}><td>{database ? `${database.label}${database.is_enabled ? '' : ' (disabled)'}` : grant.database_id}</td><td><Badge value={grant.access_level} /></td><td><div className="toolbar-actions"><button className="button small" onClick={() => setEditor({ kind: 'grant', group, grant })}>Edit</button><ActionButton className="button small danger" onAction={async () => {
              const retained = database ? members.map(user => { const access = userAccess(data, user, database, group.id); return `${user.username}: ${accessLabel(access.level)}${access.sources.length ? ` via ${access.sources.join(', ')}` : ''}` }).join('; ') : ''
              if (!await confirm({ title: `Revoke ${group.name} access to ${database?.label || grant.database_id}?`, description: `Members may retain access through other groups or administrator roles. After revoking: ${retained || 'No current members or enabled connection'}. Affected sessions will refresh.`, label: 'Revoke access' })) return
              await api(`/api/v1/admin/groups/${encode(group.id)}/databases/${encode(grant.database_id)}/access`, { method: 'DELETE' })
              await changed(`Access to ${database?.label || grant.database_id} revoked for ${group.name}.`)
            }}>Revoke</ActionButton></div></td></tr>
          })}{!grants.length && <EmptyRow columns={3}>No database access yet.</EmptyRow>}</tbody></table></div>
          <p className="helper-text">Read only: inspect schemas, browse rows, and run read-only queries. Read + migrations also permits creating, editing, and executing migrations, including schema and data changes. The SQL query editor remains read only.</p>
        </section>
        <section role="tabpanel" aria-label="Effective access" hidden={tab !== 'effective'}>
          <p className="helper-text">Effective access for this group's members across enabled databases. The strongest group permission applies. Platform administrators retain read and migration access. Inactive accounts cannot sign in.</p>
          <div className="table-scroll"><table><thead><tr><th>User</th><th>Database</th><th>Effective access</th><th>Provided by</th></tr></thead><tbody>{members.flatMap(user => data.databases.filter(database => database.is_enabled).map(database => {
            const access = userAccess(data, user, database)
            return <tr key={`${user.id}-${database.id}`}><td>{user.username}</td><td>{database.label}</td><td>{accessLabel(access.level)}</td><td>{access.sources.length ? access.sources.map(source => <span className="access-source" key={source}>{source}</span>) : 'No group grant'}</td></tr>
          }))}{(!members.length || !data.databases.some(database => database.is_enabled)) && <EmptyRow columns={4}>Add members and connections to inspect effective access.</EmptyRow>}</tbody></table></div>
        </section>
      </section>
    </div>}
    {editor && (editor.kind === 'group' || data) && <GroupEditor key={`${editor.kind}-${editor.group?.id || 'new'}`} editor={editor} data={data} onClose={() => setEditor(null)} onSaved={async (message, id) => {
      window.dispatchEvent(new Event('discard-changes'))
      setEditor(null)
      if (id) setSearch({ group: id })
      await changed(message)
    }} />}
  </>
}

function GroupEditor({ editor, data, onClose, onSaved }: { editor: Editor; data?: GroupData; onClose: () => void; onSaved: (message: string, id?: string) => Promise<void> }) {
  const grant = editor.kind === 'grant' ? editor.grant : undefined
  const initial = { name: editor.kind === 'group' ? editor.group?.name || '' : '', user_id: '', database_id: grant?.database_id || '', access_level: grant?.access_level || 'VIEW_ONLY' as AccessLevel }
  const [values, setValues] = useState(initial)
  const saved = useRef(JSON.stringify(initial))
  const action = useFormAction()
  const availableUsers = (data?.users || []).filter(user => user.is_active && !data?.members.some(member => member.group_id === editor.group?.id && member.user_id === user.id))
  const availableDatabases = (data?.databases || []).filter(database => grant ? database.id === grant.database_id : database.is_enabled && !data?.grants.some(item => item.group_id === editor.group?.id && item.database_id === database.id))
  const title = editor.kind === 'group' ? editor.group ? 'Rename group' : 'New group' : editor.kind === 'member' ? 'Add member' : grant ? 'Edit database access' : 'Add database access'
  return <Modal title={title} onClose={onClose} busy={action.busy} dirty={saved.current !== JSON.stringify(values)}>
    {close => <form aria-busy={action.busy} onSubmit={event => {
      event.preventDefault()
      void action.run(async () => {
        if (editor.kind === 'group') {
          const result = await api<Group>(`/api/v1/admin/groups${editor.group ? `/${encode(editor.group.id)}` : ''}`, { method: editor.group ? 'PATCH' : 'POST', data: { name: values.name.trim() } })
          await onSaved(`${result.name} saved.`, result.id)
        } else if (editor.kind === 'member') {
          if (!values.user_id) throw new Error('Select a user to add.')
          await api(`/api/v1/admin/groups/${encode(editor.group.id)}/members/${encode(values.user_id)}`, { method: 'PUT' })
          await onSaved(`${data?.users.find(user => user.id === values.user_id)?.username || 'User'} added to ${editor.group.name}.`)
        } else {
          if (!values.database_id) throw new Error('Select a database.')
          await api(`/api/v1/admin/groups/${encode(editor.group.id)}/databases/${encode(values.database_id)}/access`, { method: 'PUT', data: { access_level: values.access_level } })
          await onSaved(`Database access updated for ${editor.group.name}.`)
        }
      })
    }}>
      <fieldset className="form-fields" disabled={action.busy}>
        {editor.kind === 'group' ? <><label htmlFor="group-name">Group name</label><input id="group-name" required maxLength={100} autoFocus placeholder="e.g. Developers" value={values.name} onChange={event => setValues({ ...values, name: event.target.value })} /></> : editor.kind === 'member' ? <>
          <p className="muted">Add a user to {editor.group.name}.</p><label htmlFor="member-user">Platform user</label><select id="member-user" required value={values.user_id} onChange={event => setValues({ ...values, user_id: event.target.value })}><option value="" disabled>{availableUsers.length ? 'Select a user…' : 'No users available'}</option>{availableUsers.map(user => <option value={user.id} key={user.id}>{user.username}</option>)}</select>
          <p className="helper-text">Changing membership signs the affected user out so their permissions refresh.</p>
        </> : <>
          <p className="muted">Permissions for {editor.group.name}.</p><label htmlFor="grant-database">Managed database</label><select id="grant-database" required value={values.database_id} onChange={event => setValues({ ...values, database_id: event.target.value })}><option value="" disabled>{availableDatabases.length ? 'Select a database…' : 'No databases available'}</option>{availableDatabases.map(database => <option value={database.id} key={database.id}>{database.label}</option>)}</select>
          <fieldset className="permission-options"><legend>Access level</legend>{(['VIEW_ONLY', 'VIEW_EDIT'] as const).map(level => <label className="permission-option" key={level}><input type="radio" name="access_level" value={level} checked={values.access_level === level} onChange={() => setValues({ ...values, access_level: level })} /><span>{accessLabel(level)}</span></label>)}
            <p className="helper-text">{values.access_level === 'VIEW_EDIT' ? 'Inspect schemas, browse data, run read-only queries, and create, edit, or execute migrations. Migrations can change schemas and data.' : 'Inspect schemas, browse table rows, and run permitted read-only queries. Cannot author or execute migrations.'}</p>
          </fieldset><p className="helper-text">Changes refresh affected members' sessions. Other groups may still provide broader access.</p>
        </>}
      </fieldset>
      <ErrorState error={action.error} />
      <div className="form-actions"><button className="button" type="button" disabled={action.busy} onClick={close}>Cancel</button><button className="button primary" type="submit" disabled={action.busy || (editor.kind === 'member' && !availableUsers.length) || (editor.kind === 'grant' && !availableDatabases.length)}>{action.busy ? 'Saving…' : editor.kind === 'group' ? 'Save group' : editor.kind === 'member' ? 'Add member' : 'Save access'}</button></div>
    </form>}
  </Modal>
}
