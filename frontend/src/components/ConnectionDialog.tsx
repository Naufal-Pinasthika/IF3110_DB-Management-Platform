import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, encode } from '../api'
import { useUI } from '../ui-context'
import type { ConnectionDetail, ConnectionTest } from '../types'
import { ErrorState, Loading, Modal } from './ui'
import { useFormAction } from '../form-hooks'

// Provide known targets for the local Compose stack
const composeTargets = [
  { host: 'target-db-1', engine: 'postgres', port: 5432 },
  { host: 'target-db-2', engine: 'postgres', port: 5432 },
  { host: 'target-db-3', engine: 'postgres', port: 5432 },
  { host: 'target-mysql', engine: 'mysql', port: 3306 },
] as const

const isComposeTarget = (host: string) => composeTargets.some(target => target.host === host)
const composeTarget = (host: string) => composeTargets.find(target => target.host === host)
const composeEngine = (port: string) => port === '3306' ? 'mysql' : 'postgres'

const composeTargetPort = (host: string) => {
  const target = composeTarget(host)
  return target ? String(target.port) : ''
}

// Load saved settings before opening the editable connection form
export default function ConnectionDialog({ id, onClose }: { id?: string; onClose: () => void }) {
  const connection = useQuery({
    queryKey: ['admin-connection', id],
    queryFn: ({ signal }) => api<ConnectionDetail>(`/api/v1/admin/databases/${encode(id!)}`, { signal }),
    enabled: Boolean(id),
    staleTime: 0,
  })

  if (id && !connection.data) {
    return (
      <Modal title="Edit connection" onClose={onClose}>
        {connection.isPending ? (
          <Loading>Loading connection…</Loading>
        ) : (
          <ErrorState error={connection.error} retry={() => connection.refetch()} />
        )}
      </Modal>
    )
  }

  return (
    <ConnectionForm
      key={id || 'new'}
      connection={id ? connection.data : undefined}
      onClose={onClose}
    />
  )
}

// Keep saved credentials unchanged until the user chooses to replace them
function ConnectionForm({
  connection,
  onClose,
}: {
  connection?: ConnectionDetail
  onClose: () => void
}) {
  const initial = {
    engine: connection?.engine || 'postgres',
    label: connection?.label || '',
    host: connection?.host || '',
    port: String(connection?.port || 5432),
    database_name: connection?.database_name || '',
    ssl_mode: connection?.ssl_mode || 'verify-full',
    reader_username: connection?.reader_username || '',
    reader_password: '',
    migrator_username: connection?.migrator_username || '',
    migrator_password: '',
    is_enabled: connection?.is_enabled ?? true,
    use_reader_credentials: false,
    edit_reader: !connection,
    edit_migrator: !connection,
  }

  const [values, setValues] = useState(initial)
  const [composeHost, setComposeHost] = useState(connection ? 'custom' : '')

  // Keep the first values so the dialog can warn about unsaved edits
  const [saved] = useState(() => JSON.stringify(initial))
  const [test, setTest] = useState<ConnectionTest | null>(null)
  const [showReader, setShowReader] = useState(false)
  const [showMigrator, setShowMigrator] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const action = useFormAction()
  const client = useQueryClient()
  const { notify } = useUI()

  // Reset the previous connection check after any field changes
  const update = <K extends keyof typeof values>(key: K, value: typeof values[K]) => {
    setValues(previous => ({ ...previous, [key]: value }))
    setTest(null)
  }

  // Send only credential values that the user chose to edit
  const payload = () => ({
    engine: values.engine,
    label: values.label.trim(),
    host: values.host.trim(),
    port: Number(values.port),
    database_name: values.database_name.trim(),
    ssl_mode: values.ssl_mode,
    reader_username: values.reader_username.trim(),
    ...(values.edit_reader ? { reader_password: values.reader_password } : {}),
    use_reader_credentials: values.use_reader_credentials,
    ...(!values.use_reader_credentials
      ? {
          migrator_username: values.migrator_username.trim(),
          ...(values.edit_migrator ? { migrator_password: values.migrator_password } : {}),
        }
      : {}),
    ...(connection ? { is_enabled: values.is_enabled } : {}),
  })

  const descriptions: Partial<Record<string, string>> = {
    require: "Traffic is encrypted; the server's identity is not verified.",
    'verify-ca': 'Traffic is encrypted and the server certificate authority is verified.',
  }

  // Show one password field with its own visibility and save state
  const secretField = (role: 'reader' | 'migrator') => {
    const editing = role === 'reader' ? values.edit_reader : values.edit_migrator
    const visible = role === 'reader' ? showReader : showMigrator
    const setVisible = role === 'reader' ? setShowReader : setShowMigrator

    return (
      <div className="secret-field">
        <label htmlFor={`${role}-password`}>Password</label>
        {editing ? (
          <>
            <div className="password-control">
              <input
                id={`${role}-password`}
                type={visible ? 'text' : 'password'}
                autoComplete="new-password"
                required
                value={values[`${role}_password`]}
                onChange={event => update(`${role}_password`, event.target.value)}
              />
              <button
                type="button"
                className="password-toggle"
                aria-label={`${visible ? 'Hide' : 'Show'} ${role} password`}
                aria-pressed={visible}
                onClick={() => setVisible(!visible)}
              >
                {visible ? 'Hide' : 'Show'}
              </button>
            </div>
            {connection && (
              <button
                type="button"
                className="button quiet small"
                onClick={() => {
                  update(`edit_${role}`, false)
                  update(`${role}_password`, '')
                  setVisible(false)
                }}
              >
                Keep stored password
              </button>
            )}
          </>
        ) : (
          <div className="secret-state">
            <span>Stored password unchanged</span>
            <button
              className="button small"
              type="button"
              onClick={() => update(`edit_${role}`, true)}
            >
              Change password
            </button>
          </div>
        )}
      </div>
    )
  }

  // Use one form for both creating and editing a connection
  return (
    <Modal
      title={connection ? 'Edit connection' : 'Add connection'}
      className="connection-dialog"
      onClose={onClose}
      busy={action.busy}
      dirty={JSON.stringify(values) !== saved}
    >
      {close => (
        <form
          ref={form}
          id="connection-form"
          aria-busy={action.busy}
          onSubmit={event => {
            event.preventDefault()
            const data = payload()

            void action.run(async () => {
              await api(
                `/api/v1/admin/databases${connection ? `/${encode(connection.id)}` : ''}`,
                { method: connection ? 'PATCH' : 'POST', data },
              )
              onClose()
              notify(`Connection saved. ${data.label} is ready.`)
              await client.invalidateQueries()
            })
          }}
        >
          <fieldset className="form-fields dialog-body" disabled={action.busy}>
            <section>
              <h3>Connection details</h3>
              {connection || composeHost === 'custom' ? (
                <>
                  <label htmlFor="connection-engine">Database engine</label>
                  <select
                    id="connection-engine"
                    value={values.engine}
                    onChange={event => {
                      const engine = event.target.value
                      setValues(previous => ({
                        ...previous,
                        engine,
                        port: ['5432', '3306'].includes(previous.port)
                          ? engine === 'mysql' ? '3306' : '5432'
                          : previous.port,
                        ssl_mode:
                          engine === 'mysql' && previous.ssl_mode === 'verify-ca'
                            ? 'verify-full'
                            : previous.ssl_mode,
                      }))
                      setTest(null)
                    }}
                  >
                    <option value="postgres">PostgreSQL</option>
                    <option value="mysql">MySQL</option>
                  </select>
                </>
              ) : null}
              <label htmlFor="connection-label">Connection label</label>
              <input
                id="connection-label"
                required
                autoFocus
                placeholder="e.g. Inventory"
                value={values.label}
                onChange={event => update('label', event.target.value)}
              />
              {!connection && (
                <>
                  <label htmlFor="connection-compose-host">Host</label>
                  <select
                    id="connection-compose-host"
                    required
                    value={composeHost}
                    onChange={event => {
                      const host = event.target.value
                      const target = composeTarget(host)
                      setComposeHost(host)
                      setValues(previous => ({
                        ...previous,
                        host: target?.host || '',
                        database_name: target ? 'target_db' : '',
                        ...(target ? { engine: target.engine, port: String(target.port) } : {}),
                      }))
                      setTest(null)
                    }}
                  >
                    <option value="" disabled hidden>Choose a Compose target…</option>
                    {composeTargets.map(target => (
                      <option key={target.host} value={target.host}>{target.host}</option>
                    ))}
                    <option value="custom">Custom host…</option>
                  </select>
                  {composeHost === 'custom' && (
                    <>
                      <label htmlFor="connection-custom-host">Custom host</label>
                      <input
                        id="connection-custom-host"
                        required
                        placeholder="db.example.com"
                        value={values.host}
                        onChange={event => update('host', event.target.value)}
                      />
                    </>
                  )}
                </>
              )}
              {connection && (
                <div>
                  <label htmlFor="connection-host">Host</label>
                  <input
                    id="connection-host"
                    required
                    placeholder="db.example.com"
                    value={values.host}
                    onChange={event => update('host', event.target.value)}
                  />
                </div>
              )}
              {(connection || composeHost === 'custom' || isComposeTarget(composeHost)) && (
                <div>
                  <label htmlFor="connection-port">Port</label>
                  {!connection && isComposeTarget(composeHost) ? (
                    <select
                      id="connection-port"
                      value={values.port}
                      onChange={event => {
                        const port = event.target.value
                        setValues(previous => ({ ...previous, port, engine: composeEngine(port) }))
                        setTest(null)
                      }}
                    >
                      <option value={composeTargetPort(composeHost)}>
                        {composeTargetPort(composeHost)}
                      </option>
                    </select>
                  ) : (
                    <input
                      id="connection-port"
                      required
                      type="number"
                      min={1}
                      max={65535}
                      value={values.port}
                      onChange={event => update('port', event.target.value)}
                    />
                  )}
                </div>
              )}
              {(connection || composeHost === 'custom' || isComposeTarget(composeHost)) && (
                <>
                  <label htmlFor="connection-database">Database name</label>
                  {!connection && isComposeTarget(composeHost) ? (
                    <select
                      id="connection-database"
                      value={values.database_name}
                      onChange={event => update('database_name', event.target.value)}
                    >
                      <option value="target_db">target_db</option>
                    </select>
                  ) : (
                    <input
                      id="connection-database"
                      required
                      placeholder="inventory"
                      value={values.database_name}
                      onChange={event => update('database_name', event.target.value)}
                    />
                  )}
                </>
              )}
            </section>
            <section className="connection-section">
              <h3>Credentials</h3>
              <h4>Reader</h4>
              <div className="form-columns">
                <div>
                  <label htmlFor="reader-username">Username</label>
                  <input
                    id="reader-username"
                    autoComplete="off"
                    required
                    value={values.reader_username}
                    onChange={event => update('reader_username', event.target.value)}
                  />
                </div>
                {secretField('reader')}
              </div>
              <div className="credential-heading">
                <h4>Migrator</h4>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={values.use_reader_credentials}
                    onChange={event => update('use_reader_credentials', event.target.checked)}
                  />
                  Use reader credentials
                </label>
              </div>
              {values.use_reader_credentials ? (
                <p className="helper-text">
                  The migrator will use the reader's database account and password. That account
                  needs migration privileges; its database privileges do not become read only.
                </p>
              ) : (
                <div className="form-columns">
                  <div>
                    <label htmlFor="migrator-username">Username</label>
                    <input
                      id="migrator-username"
                      autoComplete="off"
                      required
                      value={values.migrator_username}
                      onChange={event => update('migrator_username', event.target.value)}
                    />
                  </div>
                  {secretField('migrator')}
                </div>
              )}
            </section>
            <section className="connection-section">
              <h3>Advanced settings</h3>
              <label htmlFor="connection-ssl">SSL mode</label>
              <select
                id="connection-ssl"
                value={values.ssl_mode}
                onChange={event => update('ssl_mode', event.target.value)}
                aria-describedby={
                  descriptions[values.ssl_mode] ? 'ssl-description' : undefined
                }
              >
                <option value="verify-full">Verify full</option>
                {values.engine !== 'mysql' && <option value="verify-ca">Verify CA</option>}
                <option value="require">Require</option>
                <option value="disable">Disable</option>
              </select>
              {descriptions[values.ssl_mode] && (
                <p id="ssl-description" className="helper-text">
                  {descriptions[values.ssl_mode]}
                </p>
              )}
              {connection && (
                <>
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={values.is_enabled}
                      onChange={event => update('is_enabled', event.target.checked)}
                    />
                    Connection enabled
                  </label>
                  <p className="helper-text">
                    Changing the host, credentials, or enabled status affects future database
                    operations. Test these settings before saving.
                  </p>
                </>
              )}
            </section>
          </fieldset>
          <div className="connection-footer">
            <ErrorState error={action.error} />
            {test && (
              <p className={`test-result ${test.connected ? 'success' : 'error'}`} role="status">
                {test.message ||
                  (test.connected
                    ? 'Reader and migrator connections succeeded.'
                    : 'Connection failed. Check the host and credentials.')}
              </p>
            )}
            <div className="form-actions">
              <button
                type="button"
                className="button"
                disabled={action.busy}
                onClick={() => {
                  if (!form.current?.reportValidity()) return
                  setTest(null)
                  const data = {
                    ...payload(),
                    ...(connection ? { database_id: connection.id } : {}),
                  }
                  void action.run(async () =>
                    setTest(
                      await api<ConnectionTest>('/api/v1/admin/databases/test-connection', {
                        method: 'POST',
                        data,
                      }),
                    ),
                  )
                }}
              >
                {action.busy ? 'Working…' : 'Test connection'}
              </button>
              <div className="toolbar-actions">
                <button type="button" className="button" disabled={action.busy} onClick={close}>
                  Cancel
                </button>
                <button className="button primary" type="submit" disabled={action.busy}>
                  Save connection
                </button>
              </div>
            </div>
          </div>
        </form>
      )}
    </Modal>
  )
}
