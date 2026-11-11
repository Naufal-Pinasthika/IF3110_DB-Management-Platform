import { useQueryClient, useQuery } from '@tanstack/react-query'
import { Link, useSearchParams, useParams } from 'react-router'
import { useState } from 'react'
import { api, databaseAPI, databaseURL, encode, qualifiedName } from '../api'
import { useUI } from '../ui-context'
import type { TableDetail, TableRows } from '../types'
import { useDatabase, useSchemas, useTables } from '../database-hooks'
import { ActionButton, DataTable, EmptyRow, ErrorState, Loading, PageHeading, Tabs } from '../components/ui'

export function Explorer() {
  const database = useDatabase()
  const schemas = useSchemas(database.id)
  const [search, setSearch] = useSearchParams()
  const schema = schemas.data?.some(item => item.name === search.get('schema')) ? search.get('schema')! : schemas.data?.[0]?.name || ''
  const tables = useTables(database.id, schema)
  const client = useQueryClient()
  return <>
    <PageHeading title="Schema browser"><ActionButton onAction={() => client.invalidateQueries({ predicate: query => query.queryKey.includes(database.id) })}>Refresh</ActionButton></PageHeading>
    <ErrorState error={schemas.error || tables.error} retry={() => { void schemas.refetch(); if (schema) void tables.refetch() }} />
    <section className="work-surface"><div className="section-toolbar"><h2>{schema || 'Tables'}</h2><div className="toolbar-actions">
      <label htmlFor="schema-select">Schema</label><select id="schema-select" disabled={!schemas.data?.length} value={schema} onChange={event => setSearch({ schema: event.target.value })}>{(schemas.data || []).map(item => <option key={item.name}>{item.name}</option>)}</select>
      <span className="muted">{tables.data ? `${tables.data.length} table${tables.data.length === 1 ? '' : 's'}` : schemas.isPending || (schema && tables.isPending) ? 'Loading tables…' : 'No schema selected.'}</span>
    </div></div><div className="table-scroll"><table><thead><tr><th>Table</th><th>Schema</th><th /></tr></thead><tbody>
      {(tables.data || []).map(table => <tr key={table.name}><td><Link to={`${databaseURL(database.id)}/schemas/${encode(schema)}/tables/${encode(table.name)}`}>{table.name}</Link></td><td className="mono">{schema}</td><td>Table</td></tr>)}
      {!tables.data?.length && <EmptyRow columns={3}>{schemas.isPending || (schema && tables.isPending) ? 'Loading tables…' : 'This schema has no visible tables.'}</EmptyRow>}
    </tbody></table></div></section>
  </>
}

export function TableViewer() {
  const { schema = '', table = '' } = useParams()
  const database = useDatabase()
  return <TableInspector key={`${database.id}/${schema}/${table}`} schema={schema} table={table} />
}

function TableInspector({ schema, table }: { schema: string; table: string }) {
  const database = useDatabase()
  const [search, setSearch] = useSearchParams()
  const offset = Math.max(0, Number(search.get('offset')) || 0)
  const limit = [25, 50, 100].includes(Number(search.get('limit'))) ? Number(search.get('limit')) : 50
  const [tab, setTab] = useState('data')
  const { notify } = useUI()
  const endpoint = `${databaseAPI(database.id)}/schemas/${encode(schema)}/tables/${encode(table)}`
  const detail = useQuery({ queryKey: ['table', database.id, schema, table], queryFn: ({ signal }) => api<TableDetail>(endpoint, { signal }) })
  const rows = useQuery({ queryKey: ['rows', database.id, schema, table, offset, limit], queryFn: ({ signal }) => api<TableRows>(`${endpoint}/rows?limit=${limit}&offset=${offset}`, { signal }) })
  const columns = detail.data?.columns || []
  const constraints = detail.data?.constraints || []
  const indexes = detail.data?.indexes || []
  const pagination = (next: number, size = limit) => setSearch({ offset: String(next), limit: String(size) })
  return <>
    <PageHeading title={`${schema}.${table}`}>
      <ActionButton onAction={() => Promise.all([detail.refetch(), rows.refetch()])} disabled={detail.isFetching || rows.isFetching}>Refresh</ActionButton>
      <ActionButton onAction={async () => { const name = qualifiedName(schema, table, database.engine); try { await navigator.clipboard.writeText(name); notify(`Copied ${name}`) } catch { notify(`Could not copy automatically. Qualified name: ${name}`, 'error') } }}>Copy name</ActionButton>
      <Link className="button small" to={`${databaseURL(database.id)}/query?${new URLSearchParams({ schema, table })}`}>Open in SQL</Link>
    </PageHeading>
    <ErrorState error={detail.error || rows.error} retry={() => { void detail.refetch(); void rows.refetch() }} />
    <div className="inspector-surface"><Tabs tabs={[{ id: 'data', label: 'Data' }, { id: 'structure', label: 'Structure' }]} selected={tab} onSelect={setTab} label="Table details" />
      <section hidden={tab !== 'data'} role="tabpanel" aria-label="Data">
        <div className="grid-guidance"><span>Read-only data</span><span>Scroll horizontally for more columns</span></div>
        <div className="table-scroll" tabIndex={0} aria-label="Table data" aria-busy={rows.isFetching}>{rows.isPending ? <Loading>Loading table data…</Loading> : rows.data && <DataTable columns={rows.data.columns} rows={rows.data.rows} />}</div>
        <div className="pagination"><span>{rows.data?.rows?.length ? `Rows ${offset + 1}–${offset + rows.data.rows.length}` : 'No rows'}</span><div className="toolbar-actions"><label htmlFor="page-size">Rows</label><select id="page-size" value={limit} onChange={event => pagination(0, Number(event.target.value))}><option>25</option><option>50</option><option>100</option></select><button className="button small" disabled={offset === 0 || rows.isFetching} onClick={() => pagination(Math.max(0, offset - limit))}>Previous</button><button className="button small" disabled={!rows.data?.has_more || rows.isFetching} onClick={() => pagination(offset + limit)}>Next</button></div></div>
      </section>
      <section hidden={tab !== 'structure'} role="tabpanel" aria-label="Structure">
        {detail.isPending && <Loading>Loading table structure…</Loading>}
        <section className="metadata-section"><h2>Columns</h2><div className="table-scroll" tabIndex={0}><table><thead><tr><th>Column</th><th>Type</th><th>Nullable</th><th>Key</th></tr></thead><tbody>{columns.map(column => <tr key={column.name}><td className="mono">{column.name}</td><td>{column.data_type}</td><td>{column.nullable ? 'Yes' : 'No'}</td><td className="key-marker">{constraints.filter(constraint => constraint.columns?.includes(column.name)).map(constraint => ({ 'PRIMARY KEY': 'PK', 'FOREIGN KEY': 'FK', UNIQUE: 'Unique' } as Record<string, string>)[constraint.type.toUpperCase()] || constraint.type).join(', ') || '--'}</td></tr>)}{!columns.length && <EmptyRow columns={4}>No column metadata.</EmptyRow>}</tbody></table></div></section>
        <section className="metadata-section"><h2>Constraints</h2><div className="table-scroll" tabIndex={0}><table><thead><tr><th>Name</th><th>Type</th><th>Columns</th></tr></thead><tbody>{constraints.map(constraint => <tr key={constraint.name}><td>{constraint.name}</td><td>{constraint.type}</td><td>{(constraint.columns || []).join(', ')}</td></tr>)}{!constraints.length && <EmptyRow columns={3}>No constraints.</EmptyRow>}</tbody></table></div></section>
        <section className="metadata-section"><h2>Indexes</h2><div className="table-scroll" tabIndex={0}><table><thead><tr><th>Name</th><th>Columns</th><th>Unique</th></tr></thead><tbody>{indexes.map(index => <tr key={index.name}><td>{index.name}</td><td>{(index.columns || []).join(', ')}</td><td>{index.is_unique ? 'Yes' : 'No'}</td></tr>)}{!indexes.length && <EmptyRow columns={3}>No indexes.</EmptyRow>}</tbody></table></div></section>
      </section>
    </div>
  </>
}
