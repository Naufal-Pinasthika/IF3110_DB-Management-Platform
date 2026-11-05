import type { ExplainNode, ExplainResult } from '../types'

function PlanNode({ plan, depth = 0 }: { plan: ExplainNode; depth?: number }) {
  const metrics = [
    plan.join_type && `Join: ${plan.join_type}`,
    `Cost: ${Number(plan.startup_cost || 0).toFixed(2)}–${Number(plan.total_cost || 0).toFixed(2)}`,
    `Est. rows: ${plan.plan_rows ?? 0}`,
    plan.actual_rows != null && `Actual rows × loops: ${plan.actual_rows} × ${plan.actual_loops || 1}`,
    plan.actual_total_time != null && `Time: ${Number(plan.actual_total_time).toFixed(3)} ms`,
    (plan.shared_hit_blocks != null || plan.shared_read_blocks != null) && `Buffers hit/read: ${plan.shared_hit_blocks || 0}/${plan.shared_read_blocks || 0}`,
  ].filter(Boolean)
  return <>
    <div className={`plan-node plan-depth-${Math.min(depth, 8)}`}>
      <strong>{plan.node_type || 'Plan'}{plan.relation_name && ` - ${plan.schema ? `${plan.schema}.` : ''}${plan.relation_name}`}{plan.index_name && ` - ${plan.index_name}`}</strong>
      <div className="plan-metrics">{metrics.map((metric, index) => <span className="plan-metric" key={index}>{metric}</span>)}</div>
      {plan.filter && <code className="plan-filter">Filter: {plan.filter}</code>}
    </div>
    {(plan.plans || []).map((child, index) => <PlanNode plan={child} depth={depth + 1} key={index} />)}
  </>
}

export default function Plans({ result }: { result: ExplainResult }) {
  if (!result.plans?.length) return <p className="inline-state">{result.raw_text || 'The database did not produce an execution plan for these statements.'}</p>
  return <div className="plan-list">{result.plans.map((plan, index) => <article className="plan-statement" key={index}>
    {(result.plans.length > 1 || plan.statement_index) && <h3>Statement {plan.statement_index || index + 1}</h3>}
    {plan.error ? <p className="inline-state">No execution plan: {plan.error}</p> : <PlanNode plan={plan.plan} />}
    <p className="plan-total">{[plan.planning_time != null && `Planning ${plan.planning_time.toFixed(3)} ms`, plan.execution_time != null && `Execution ${plan.execution_time.toFixed(3)} ms`].filter(Boolean).join(' - ')}</p>
  </article>)}</div>
}
