import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { api, APIError } from './api'
import type { User } from './types'

// Load the current user and remove private data after the session expires
export function useSession() {
  const client = useQueryClient()
  return useQuery({
    queryKey: ['session'],
    queryFn: async ({ signal }): Promise<User | null> => {
      try {
        return await api<User>('/api/v1/auth/me', { signal })
      } catch (error) {
        // A guest session is a normal result so route guards can send users to login
        if (error instanceof APIError && error.status === 401) {
          window.dispatchEvent(new Event('discard-changes'))
          client.removeQueries({ predicate: query => query.queryKey[0] !== 'session' })
          return null
        }
        throw error
      }
    },
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  })
}

// Replace the user while keeping the query watched by mounted route guards
export async function resetSession(client: QueryClient, user: User | null) {
  await client.cancelQueries()
  client.setQueryData<User | null>(['session'], user)
  client.removeQueries({ predicate: query => query.queryKey[0] !== 'session' })
}

