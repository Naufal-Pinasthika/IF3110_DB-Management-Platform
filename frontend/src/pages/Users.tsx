import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, encode } from '../api'
import { useUI } from '../ui-context'
import type { AdminUser } from '../types'
import { ActionButton, Badge, EmptyRow, ErrorState, Modal, PageHeading } from '../components/ui'
import { useFormAction } from '../form-hooks'

export default function Users() {
  const users = useQuery({ queryKey: ['admin-users'], queryFn: ({ signal }) => api<AdminUser[]>('/api/v1/admin/users', { signal }) })
  const [editing, setEditing] = useState<AdminUser | 'new' | null>(null)
  return <>
    <PageHeading title="Users"><button className="button primary" onClick={() => setEditing('new')}>New user</button></PageHeading>
    <ErrorState error={users.error} retry={() => users.refetch()} />
    <section className="panel"><div className="panel-toolbar"><h2>Platform accounts</h2><span className="muted">Database access is assigned through groups.</span><ActionButton onAction={() => users.refetch()}>Refresh</ActionButton></div>
      <div className="table-scroll"><table><thead><tr><th>Username</th><th>Platform role</th><th>Status</th><th>Actions</th></tr></thead><tbody>
        {(users.data || []).map(user => <tr key={user.id}><td><strong>{user.username}</strong></td><td>{user.is_platform_admin ? 'Administrator' : 'User'}</td><td><Badge value={user.is_active ? 'ACTIVE' : 'DISABLED'} /></td><td><button className="button small" onClick={() => setEditing(user)}>Edit</button></td></tr>)}
        {!users.data?.length && <EmptyRow columns={4}>{users.isPending ? 'Loading users…' : 'No users found.'}</EmptyRow>}
      </tbody></table></div>
    </section>
    {editing && <UserDialog user={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} />}
  </>
}

function UserDialog({ user, onClose }: { user?: AdminUser; onClose: () => void }) {
  const initial = { username: user?.username || '', password: '', is_active: user?.is_active ?? true, is_platform_admin: false }
  const [values, setValues] = useState(initial)
  const [saved] = useState(() => JSON.stringify(initial))
  const action = useFormAction()
  const client = useQueryClient()
  const { confirm, notify } = useUI()
  return <Modal title={user ? 'Edit user' : 'New user'} onClose={onClose} dirty={saved !== JSON.stringify(values)} busy={action.busy}>
    {close => <form id="user-form" aria-busy={action.busy} onSubmit={event => {
      event.preventDefault()
      void action.run(async () => {
        if (new TextEncoder().encode(values.password).length > 72) throw new Error('Password must be no more than 72 bytes.')
        if (user && !values.is_active && !await confirm({ title: 'Disable this account?', description: 'The user will be signed out and will no longer be able to sign in.', label: 'Disable account' })) return
        await api(`/api/v1/admin/users${user ? `/${encode(user.id)}` : ''}`, { method: user ? 'PATCH' : 'POST', data: {
          username: values.username,
          ...(!user || values.password ? { password: values.password } : {}),
          ...(user ? { is_active: values.is_active } : { is_platform_admin: values.is_platform_admin }),
        } })
        onClose(); notify(user ? 'User updated.' : 'User created.')
        await client.invalidateQueries()
      })
    }}>
      <fieldset className="form-fields" disabled={action.busy}>
        <label htmlFor="user-username">Username</label><input id="user-username" maxLength={64} required autoComplete="off" autoFocus value={values.username} onChange={event => setValues({ ...values, username: event.target.value })} />
        <label htmlFor="user-password">Password</label><input id="user-password" type="password" autoComplete="new-password" required={!user} value={values.password} onChange={event => setValues({ ...values, password: event.target.value })} />
        <p className="helper-text">{user ? 'Leave blank to keep the current password.' : 'Use a password of up to 72 bytes.'}</p>
        {user ? <label className="checkbox-label"><input type="checkbox" checked={values.is_active} onChange={event => setValues({ ...values, is_active: event.target.checked })} />Account active</label> : <label className="checkbox-label"><input type="checkbox" checked={values.is_platform_admin} onChange={event => setValues({ ...values, is_platform_admin: event.target.checked })} />Platform administrator</label>}
        <p className="helper-text">Disabling an account or changing its password revokes its sessions.</p>
      </fieldset>
      <ErrorState error={action.error} />
      <div className="form-actions"><button className="button" type="button" disabled={action.busy} onClick={close}>Cancel</button><button className="button primary" type="submit" disabled={action.busy}>{action.busy ? 'Saving…' : 'Save user'}</button></div>
    </form>}
  </Modal>
}
