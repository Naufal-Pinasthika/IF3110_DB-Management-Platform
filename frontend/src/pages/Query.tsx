import { useEffect, useRef, useState } from 'react'
import { useLocation, useSearchParams } from 'react-router'
import { api, databaseAPI, errorMessage, qualifiedName } from '../api'
import { useDatabase } from '../components/layout'
import { DataTable, ErrorState, PageHeading, useFormAction } from '../components/ui'
import Plans from '../components/Plans'
import type { ExplainResult, QueryResult } from '../types'

export default function QueryRoute() {
  const location = useLocation()
  return <QueryPage key={location.pathname + location.search} />
}

function QueryPage() {
  const database = useDatabase()
  const [search] = useSearchParams()
  const [sql, setSQL] = useState(() => search.has('schema') && search.has('table') ? `SELECT * FROM ${qualifiedName(search.get('schema')!, search.get('table')!, database.engine)} LIMIT 50;` : '')
  const [limit, setLimit] = useState(50)
  const [result, setResult] = useState<QueryResult | null>(null)
  const [plan, setPlan] = useState<ExplainResult | null>(null)
  const [planError, setPlanError] = useState('')
  const [summary, setSummary] = useState('No query run')
  const action = useFormAction()
  const controller = useRef<AbortController | null>(null)
  const results = useRef<HTMLDivElement>(null)
  useEffect(() => () => controller.current?.abort(), [])
  return <>
    <PageHeading title="SQL query" />
    <form id="query-form" className="work-surface query-surface" aria-busy={action.busy} onSubmit={event => {
      event.preventDefault()
      if (!sql.trim()) return
      void action.run(async () => {
        const abort = new AbortController()
        controller.current = abort
        const started = performance.now()
        setSummary('Running…'); setPlan(null); setPlanError(''); setResult(null)
        try {
          const data = await api<QueryResult>(`${databaseAPI(database.id)}/queries`, { method: 'POST', data: { sql, max_rows: limit }, signal: abort.signal })
          setResult(data)
          setSummary(`${data.row_count} row${data.row_count === 1 ? '' : 's'} - ${((performance.now() - started) / 1000).toFixed(2)}s${data.truncated ? ' - Row limit reached' : ''}`)
          if (database.engine === 'mysql') setPlanError('Execution plan analysis is not available for MySQL targets yet.')
          else {
            try { setPlan(await api<ExplainResult>(`${databaseAPI(database.id)}/queries/explain`, { method: 'POST', data: { sql, analyze: true }, signal: abort.signal })) }
            catch (error) { if (!abort.signal.aborted) setPlanError(errorMessage(error)) }
          }
          results.current?.focus()
        } catch (error) {
          if (!abort.signal.aborted) { setSummary(`Query failed - ${((performance.now() - started) / 1000).toFixed(2)}s`); throw error }
        }
      })
    }}>
      <div className="section-toolbar"><h2>Query</h2><span className="muted">SELECT only</span></div>
      <label className="sr-only" htmlFor="sql">SQL query</label><textarea id="sql" name="sql" className="sql-editor" spellCheck={false} required disabled={action.busy} value={sql} onChange={event => setSQL(event.target.value)} placeholder={database.engine === 'mysql' ? 'SELECT * FROM `your_database`.`your_table` LIMIT 50;' : 'SELECT * FROM public.your_table LIMIT 50;'} onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); if (!action.busy) event.currentTarget.form?.requestSubmit() } }} />
      <div className="editor-footer"><span className="execution-target">Target: <code>{database.database_name}</code></span><div className="toolbar-actions"><span className="shortcut">Ctrl / ⌘ + Enter</span><label htmlFor="max-rows">Limit</label><select id="max-rows" value={limit} disabled={action.busy} onChange={event => setLimit(Number(event.target.value))}><option>25</option><option>50</option><option>100</option></select><button type="submit" className="button primary" disabled={action.busy}>{action.busy ? 'Running…' : 'Run query'}</button></div></div>
      <ErrorState error={action.error} />
    </form>
    <section className="query-results-surface" aria-label="Execution plan"><div className="section-toolbar"><h2>Execution plan</h2><span className="muted" role="status">{plan ? 'EXPLAIN ANALYZE - rolled back - actual timings and rows' : action.busy ? 'Analyzing…' : 'Plan appears after a query runs.'}</span></div>{plan && <Plans result={plan} />}{planError && <p className="inline-state">{planError}</p>}</section>
    <section className="query-results-surface" aria-label="Query results"><div className="section-toolbar"><h2>Results</h2><span className="muted" role="status">{summary}</span></div><div ref={results} className="table-scroll" tabIndex={0} aria-busy={action.busy}>{result ? <DataTable columns={result.columns} rows={result.rows} /> : <p className="inline-state">{action.busy ? 'Running query…' : 'Run a SELECT query to view results.'}</p>}</div></section>
  </>
}
