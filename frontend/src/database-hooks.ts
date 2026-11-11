import { useQuery } from '@tanstack/react-query'
import { useOutletContext } from 'react-router'
import { api, databaseAPI, encode } from './api'
import type { Database, NamedObject } from './types'

export function useDatabaseQuery(id: string) {
  return useQuery({ queryKey: ['database', id], queryFn: ({ signal }) => api<Database>(databaseAPI(id), { signal }), enabled: Boolean(id) })
}
export function useDatabase() { return useOutletContext<Database>() }
export function useSchemas(id: string) {
  return useQuery({ queryKey: ['schemas', id], queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas`, { signal }) })
}
export function useTables(id: string, schema: string, enabled = true) {
  return useQuery({ queryKey: ['tables', id, schema], queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas/${encode(schema)}/tables`, { signal }), enabled: Boolean(schema) && enabled })
}

