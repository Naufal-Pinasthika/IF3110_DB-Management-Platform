export type AccessLevel = 'VIEW_ONLY' | 'VIEW_EDIT'
export interface User { id: string; username: string; is_platform_admin: boolean }
export interface AdminUser extends User { is_active: boolean }
export interface Group { id: string; name: string }
export interface Member { group_id: string; user_id: string }
export interface Grant { group_id: string; database_id: string; access_level: AccessLevel }
export interface AccessSnapshot { members: Member[]; grants: Grant[] }
export interface Database {
  id: string; engine: string; label: string; database_name: string; effective_access: AccessLevel
}
export interface Connection {
  id: string; engine: string; label: string; database_name: string; is_enabled: boolean
}
export interface ConnectionDetail extends Connection {
  host: string; port: number; ssl_mode: string; reader_username: string; migrator_username: string
}
export interface NamedObject { name: string }
export interface Constraint { name: string; type: string; columns: string[] }
export interface TableDetail {
  schema: string; table: string
  columns: { name: string; data_type: string; nullable: boolean }[]
  constraints: Constraint[]
  indexes: { name: string; columns: string[]; is_unique: boolean }[]
}
export interface TableRows { columns: string[]; rows: unknown[][]; limit: number; offset: number; has_more: boolean }
export interface QueryResult { columns: string[]; rows: unknown[][]; row_count: number; truncated: boolean }
export interface ExplainNode {
  node_type: string; relation_name?: string; schema?: string; index_name?: string; join_type?: string
  startup_cost: number; total_cost: number; plan_rows: number; actual_rows?: number; actual_loops?: number
  actual_total_time?: number; shared_hit_blocks?: number; shared_read_blocks?: number; filter?: string; plans?: ExplainNode[]
}
export interface ExplainResult {
  plans: { plan: ExplainNode; statement_index?: number; error?: string; planning_time?: number; execution_time?: number }[]
  raw_text: string; analyzed: boolean
}
export interface MigrationSummary {
  id: string; name: string; version: number | null; status: string; executed_by: string | null; executed_at: string | null
}
export interface Migration extends MigrationSummary {
  database_id: string; sql_text: string; created_by: string; created_at: string; updated_at: string; error_summary: string | null
}
export interface SchemaDiff {
  has_changes: boolean; generated_sql: string; excluded_object_count: number
  changes: { kind: string; action: string; schema: string; table?: string; name: string; before?: string; after?: string }[]
}
export interface ConnectionTest { connected: boolean; message?: string }
